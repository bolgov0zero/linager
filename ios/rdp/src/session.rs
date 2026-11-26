use std::cell::RefCell;
use std::io;
use std::pin::Pin;
use std::rc::Rc;
use std::task::{Context, Poll};

use anyhow::Context as _;
use futures_channel::mpsc::UnboundedReceiver;
use futures_util::io::{AsyncRead, AsyncWrite};
use futures_util::{select, FutureExt as _, StreamExt as _};
use ironrdp::connector::sspi::generator::NetworkRequest;
use ironrdp::connector::{self, ClientConnector, Credentials, Sequence as _};
use ironrdp::graphics::image_processing::PixelFormat;
use ironrdp::input::{Database, MouseButton, MousePosition, Operation, Scancode, WheelRotations};
use ironrdp::dvc::DrdynvcClient;
use ironrdp::pdu::rdp::capability_sets::client_codecs_capabilities;
use ironrdp_egfx::client::{GraphicsPipelineClient, GraphicsPipelineHandler};
use ironrdp::pdu::rdp::client_info::{PerformanceFlags, TimezoneInfo};
use ironrdp::session::image::DecodedImage;
use ironrdp::session::{ActiveStageBuilder, ActiveStageOutput};
use ironrdp_core::WriteBuf;
use ironrdp_futures::{FramedWrite as _, LocalFuturesFramed, NetworkClient};

pub struct Params {
    pub username: String,
    pub password: String,
    pub domain: Option<String>,
    /// "address:port" of the computer as the server knows it.
    pub destination: String,
    pub proxy_auth: String,
    pub width: u16,
    pub height: u16,
}

/// What the application is told.
pub enum Event<'a> {
    Connected { width: u16, height: u16 },
    /// A changed rectangle with its pixels (RGBA, rows packed).
    Frame { x: u16, y: u16, w: u16, h: u16, data: &'a [u8] },
    /// The session is over; the text is empty for a plain end.
    Ended(String),
}

/// What the application asks.
pub enum Command {
    Input(Vec<Operation>),
    Close,
}

/// The two ways out of the core: one message for the WebSocket, one event for the application. Both are called on the thread of the session.
pub struct Sink {
    pub send: Rc<RefCell<dyn FnMut(&[u8])>>,
    pub event: Box<dyn FnMut(Event<'_>)>,
}

/// The messages of the WebSocket as a byte stream: reads come from the channel the application feeds, writes go to the send callback.
struct Stream {
    rx: UnboundedReceiver<Vec<u8>>,
    send: Rc<RefCell<dyn FnMut(&[u8])>>,
    buf: Vec<u8>,
    pos: usize,
}

impl AsyncRead for Stream {
    fn poll_read(mut self: Pin<&mut Self>, cx: &mut Context<'_>, out: &mut [u8]) -> Poll<io::Result<usize>> {
        loop {
            if self.pos < self.buf.len() {
                let n = out.len().min(self.buf.len() - self.pos);
                out[..n].copy_from_slice(&self.buf[self.pos..self.pos + n]);
                self.pos += n;
                return Poll::Ready(Ok(n));
            }
            match self.rx.poll_next_unpin(cx) {
                Poll::Ready(Some(v)) => {
                    self.buf = v;
                    self.pos = 0;
                }
                Poll::Ready(None) => return Poll::Ready(Ok(0)),
                Poll::Pending => return Poll::Pending,
            }
        }
    }
}

impl AsyncWrite for Stream {
    fn poll_write(self: Pin<&mut Self>, _: &mut Context<'_>, data: &[u8]) -> Poll<io::Result<usize>> {
        (self.send.borrow_mut())(data);
        Poll::Ready(Ok(data.len()))
    }
    fn poll_flush(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }
    fn poll_close(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }
}

/// No Kerberos: a sign-in with a user name and a password goes by NTLM, which needs no network.
struct NoNetwork;
impl NetworkClient for NoNetwork {
    async fn send(&mut self, _: &NetworkRequest) -> connector::ConnectorResult<Vec<u8>> {
        Err(connector::reason_err!("network", "no network requests"))
    }
}

fn config(p: &Params) -> connector::Config {
    connector::Config {
        credentials: Credentials::UsernamePassword { username: p.username.clone(), password: p.password.clone() },
        domain: p.domain.clone(),
        enable_tls: true,
        enable_credssp: true,
        enable_standard_rdp_security: false,
        keyboard_type: ironrdp::pdu::gcc::KeyboardType::IBM_ENHANCED,
        keyboard_subtype: 0,
        keyboard_layout: 0, // the computer uses its own active input locale
        keyboard_functional_keys_count: 12,
        connection_type: ironrdp::pdu::gcc::ConnectionType::Lan,
        ime_file_name: String::new(),
        dig_product_id: String::new(),
        desktop_size: connector::DesktopSize { width: p.width, height: p.height },
        monitor_layout: None,
        // As the browser client of the desktop application asks: 16 bits, lossy compression and the codecs of the server (RemoteFX and the
        // others), and the graphics pipeline (below). Plain 32-bit bitmaps cost several times the traffic and the work of the computer.
        bitmap: Some(connector::BitmapConfig { color_depth: 16, lossy_compression: true, codecs: client_codecs_capabilities(&[]).expect("no codec is named, so none can be wrong") }),
        client_build: 100,
        client_name: "linager-ios".to_owned(),
        client_dir: "C:\\Windows\\System32\\mstscax.dll".to_owned(),
        platform: ironrdp::pdu::rdp::capability_sets::MajorPlatformType::UNSPECIFIED,
        compression_type: None,
        enable_server_pointer: true,
        autologon: false,
        enable_audio_playback: false,
        enable_audio_capture: false,
        request_data: None,
        pointer_software_rendering: false,
        multitransport_flags: None,
        support_dyn_vc_gfx_protocol: true,
        rdp_version: ironrdp::pdu::gcc::RdpVersion::V5_PLUS,
        performance_flags: PerformanceFlags::default(),
        desktop_scale_factor: 0,
        desktop_physical_size: None,
        desktop_orientation: None,
        device_scale_factor: None,
        monitor_extended_layout: None,
        relative_mouse_input: false,
        hardware_id: None,
        license_cache: None,
        timezone_info: TimezoneInfo::default(),
        alternate_shell: String::new(),
        work_dir: String::new(),
        remote_application_mode: false,
        rail_support_level: ironrdp::pdu::rdp::capability_sets::RailSupportLevel::SUPPORTED,
    }
}

#[derive(Debug)]
struct Hint;
impl ironrdp::pdu::PduHint for Hint {
    fn find_size(&self, bytes: &[u8]) -> ironrdp::core::DecodeResult<Option<(bool, usize)>> {
        match ironrdp_rdcleanpath::RDCleanPathPdu::detect(bytes) {
            ironrdp_rdcleanpath::DetectionResult::Detected { total_length, .. } => Ok(Some((true, total_length))),
            ironrdp_rdcleanpath::DetectionResult::NotEnoughBytes => Ok(None),
            ironrdp_rdcleanpath::DetectionResult::Failed => Err(ironrdp::core::other_err!("RDCleanPathHint", "detect RDCleanPath PDU")),
        }
    }
}

struct NoGraphicsHandler;
impl GraphicsPipelineHandler for NoGraphicsHandler {}

fn err(e: impl std::fmt::Debug) -> anyhow::Error {
    anyhow::anyhow!("{e:?}")
}

async fn connect(stream: Stream, p: &Params) -> anyhow::Result<(connector::ConnectionResult, LocalFuturesFramed<Stream>)> {
    let mut framed = LocalFuturesFramed::new(stream);
    let addr = core::net::SocketAddr::V4(core::net::SocketAddrV4::new(core::net::Ipv4Addr::LOCALHOST, 33899));
    let mut connector = ClientConnector::new(config(p), addr);
    // the graphics pipeline (EGFX) runs over a dynamic channel; the session decodes it into the picture by itself. There is no H.264 decoder
    // here, so the pipeline falls back to the codecs that need none.
    connector.attach_static_channel(DrdynvcClient::new().with_dynamic_channel(GraphicsPipelineClient::new(Box::new(NoGraphicsHandler), None)));
    let server_name = connector::ServerName::from(p.destination.as_str());
    let mut net = NoNetwork;
    let mut buf = WriteBuf::new();

    let connector::ClientConnectorState::ConnectionInitiationSendRequest = connector.state else { anyhow::bail!("invalid connector state") };
    connector.step_no_input(&mut buf).map_err(err)?;
    let x224 = buf.filled().to_vec();
    let req = ironrdp_rdcleanpath::RDCleanPathPdu::new_request(x224, p.destination.clone(), p.proxy_auth.clone(), None).context("request")?;
    framed.write_all(&req.to_der().context("encode request")?).await.context("write request")?;

    let res = framed.read_by_hint(&Hint).await.context("read the answer of the server")?;
    let res = ironrdp_rdcleanpath::RDCleanPathPdu::from_der(&res).context("decode the answer")?;
    let (x224_response, chain) = match res.into_message().context("invalid answer")? {
        ironrdp_rdcleanpath::RDCleanPathMessage::Response { x224_connection_response, server_cert_chain, .. } => (x224_connection_response, server_cert_chain),
        ironrdp_rdcleanpath::RDCleanPathMessage::GeneralErr(e) => anyhow::bail!("the server could not connect: {e:?}"),
        ironrdp_rdcleanpath::RDCleanPathMessage::NegotiationErr { .. } => anyhow::bail!("the computer refused the security settings"),
        _ => anyhow::bail!("unexpected answer of the server"),
    };
    buf.clear();
    connector.step(x224_response.as_bytes(), None, &mut buf).map_err(err)?;
    let cert = chain.into_iter().next().context("no certificate in the answer")?;
    use x509_cert::der::Decode as _;
    let cert = x509_cert::Certificate::from_der(cert.as_bytes()).context("certificate")?;
    let server_public_key = cert.tbs_certificate().subject_public_key_info().subject_public_key.as_bytes().context("key")?.to_owned();

    let should_upgrade = ironrdp_futures::skip_connect_begin(&mut connector);
    let upgraded = ironrdp_futures::mark_as_upgraded(should_upgrade, &mut connector);
    let outcome = ironrdp_futures::connect_finalize(upgraded, connector, &mut framed, &mut net, server_name, server_public_key, None)
        .await
        .map_err(err)?
        .into_connected()
        .map_err(err)?;
    Ok((outcome, framed))
}

/// The whole session: connects, then carries pictures out and input in until the end. The end is reported through `Event::Ended`.
pub async fn run(p: Params, rx: UnboundedReceiver<Vec<u8>>, mut cmds: UnboundedReceiver<Command>, mut sink: Sink) {
    let result = session(&p, rx, &mut cmds, &mut sink).await;
    let text = match result {
        Ok(()) => String::new(),
        Err(e) => format!("{e:#}"),
    };
    (sink.event)(Event::Ended(text));
}

async fn session(p: &Params, rx: UnboundedReceiver<Vec<u8>>, cmds: &mut UnboundedReceiver<Command>, sink: &mut Sink) -> anyhow::Result<()> {
    let stream = Stream { rx, send: sink.send.clone(), buf: Vec::new(), pos: 0 };
    let (conn, mut framed) = connect(stream, p).await?;
    let (width, height) = (conn.desktop_size.width, conn.desktop_size.height);
    (sink.event)(Event::Connected { width, height });

    let mut image = DecodedImage::new(PixelFormat::RgbA32, width, height);
    let mut stage = ActiveStageBuilder {
        static_channels: conn.static_channels,
        user_channel_id: conn.user_channel_id,
        io_channel_id: conn.io_channel_id,
        message_channel_id: conn.message_channel_id,
        share_id: conn.share_id,
        compression_type: conn.compression_type,
        enable_server_pointer: conn.enable_server_pointer,
        pointer_software_rendering: conn.pointer_software_rendering,
    }
    .build();
    let mut input = Database::new();
    let mut piece: Vec<u8> = Vec::new();

    loop {
        let outputs = select! {
            frame = framed.read_pdu().fuse() => {
                let (action, payload) = frame.context("the connection is lost")?;
                stage.process(&mut image, action, &payload, framed.last_read_at()).map_err(err)?
            }
            cmd = cmds.next() => match cmd {
                Some(Command::Input(ops)) => {
                    let events = input.apply(ops);
                    stage.process_fastpath_input(&mut image, &events).map_err(err)?
                }
                Some(Command::Close) | None => {
                    let bye = stage.graceful_shutdown().map_err(err)?;
                    for out in bye {
                        if let ActiveStageOutput::ResponseFrame(f) = out { let _ = framed.write_all(&f).await; }
                    }
                    return Ok(());
                }
            }
        };
        for out in outputs {
            match out {
                ActiveStageOutput::ResponseFrame(f) => framed.write_all(&f).await.context("write")?,
                ActiveStageOutput::GraphicsUpdate(r) => {
                    let (w, h) = (image.width() as usize, image.height() as usize);
                    let (l, t) = (usize::from(r.left).min(w), usize::from(r.top).min(h));
                    let (rr, bb) = ((usize::from(r.right) + 1).min(w), (usize::from(r.bottom) + 1).min(h));
                    if rr <= l || bb <= t { continue; }
                    piece.clear();
                    let data = image.data();
                    for row in t..bb {
                        piece.extend_from_slice(&data[(row * w + l) * 4..(row * w + rr) * 4]);
                    }
                    (sink.event)(Event::Frame { x: l as u16, y: t as u16, w: (rr - l) as u16, h: (bb - t) as u16, data: &piece });
                }
                ActiveStageOutput::Terminate(reason) => {
                    use ironrdp::session::GracefulDisconnectReason as R;
                    return match reason {
                        R::UserInitiated => Ok(()),
                        R::ServerInitiated => anyhow::bail!("the computer ended the session"),
                        other => anyhow::bail!("the computer ended the session: {other:?}"),
                    };
                }
                ActiveStageOutput::DeactivateAll => anyhow::bail!("the computer changed its screen (resolution); connect again"),
                _ => {}
            }
        }
    }
}

// Input operations ----------------------------------------------------------------------------------------------

pub fn op_move(x: u16, y: u16) -> Operation {
    Operation::MouseMove(MousePosition { x, y })
}

pub fn op_button(button: i32, down: bool) -> Option<Operation> {
    let b = MouseButton::from_idx(usize::try_from(button).ok()?)?;
    Some(if down { Operation::MouseButtonPressed(b) } else { Operation::MouseButtonReleased(b) })
}

pub fn op_wheel(vertical: bool, notches: i32) -> Operation {
    // 120 units a notch; RDP counts a wheel turned away from the user (up) as positive, and the phone counts down as positive
    let units = (notches.clamp(-50, 50) * 120 * if vertical { -1 } else { 1 }) as i16;
    Operation::WheelRotations(WheelRotations { is_vertical: vertical, rotation_units: units })
}

pub fn op_key(scancode: u8, extended: bool, down: bool) -> Operation {
    let s = Scancode::from_u8(extended, scancode);
    if down { Operation::KeyPressed(s) } else { Operation::KeyReleased(s) }
}

pub fn op_unicode(scalar: u32, down: bool) -> Option<Operation> {
    let c = char::from_u32(scalar)?;
    Some(if down { Operation::UnicodeKeyPressed(c) } else { Operation::UnicodeKeyReleased(c) })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wheel_is_in_notches_and_up_is_positive() {
        // the phone counts down as positive, RDP counts up as positive
        match op_wheel(true, 2) { Operation::WheelRotations(w) => assert_eq!((w.is_vertical, w.rotation_units), (true, -240)), _ => panic!() }
        match op_wheel(false, 1) { Operation::WheelRotations(w) => assert_eq!((w.is_vertical, w.rotation_units), (false, 120)), _ => panic!() }
        // a wild number does not overflow
        match op_wheel(true, i32::MAX) { Operation::WheelRotations(w) => assert_eq!(w.rotation_units, -6000), _ => panic!() }
    }

    #[test]
    fn buttons_and_keys() {
        assert!(op_button(0, true).is_some() && op_button(4, false).is_some());
        assert!(op_button(5, true).is_none() && op_button(-1, true).is_none());
        match op_key(0x4b, true, true) { Operation::KeyPressed(s) => assert_eq!(s.as_u16(), 0xE04B), _ => panic!() }
        match op_key(0x1e, false, false) { Operation::KeyReleased(s) => assert_eq!(s.as_u16(), 0x1E), _ => panic!() }
    }

    #[test]
    fn unicode_takes_scalars_only() {
        assert!(op_unicode(0x44f, true).is_some()); // я
        assert!(op_unicode(0x1F600, true).is_some()); // an emoji, beyond the 16 bits
        assert!(op_unicode(0xD800, true).is_none()); // half of a surrogate pair
    }

    #[test]
    fn the_stream_reads_what_is_fed_and_writes_through_the_callback() {
        use futures_channel::mpsc::unbounded;
        use futures_util::io::{AsyncReadExt as _, AsyncWriteExt as _};
        let (tx, rx) = unbounded::<Vec<u8>>();
        let written = Rc::new(RefCell::new(Vec::<u8>::new()));
        let w = written.clone();
        let send: Rc<RefCell<dyn FnMut(&[u8])>> = Rc::new(RefCell::new(move |d: &[u8]| w.borrow_mut().extend_from_slice(d)));
        let mut s = Stream { rx, send, buf: Vec::new(), pos: 0 };
        tx.unbounded_send(vec![1, 2, 3]).unwrap();
        tx.unbounded_send(vec![4, 5]).unwrap();
        drop(tx);
        futures_executor_block(async {
            let mut out = Vec::new();
            s.read_to_end(&mut out).await.unwrap();
            assert_eq!(out, vec![1, 2, 3, 4, 5]); // messages are one stream, and the end of the feed is the end of the stream
            s.write_all(&[9, 8]).await.unwrap();
        });
        assert_eq!(*written.borrow(), vec![9, 8]);
    }

    fn futures_executor_block<F: std::future::Future>(f: F) -> F::Output {
        tokio::runtime::Builder::new_current_thread().build().unwrap().block_on(f)
    }
}
