//! The C interface (see include/linager_rdp.h). A session runs on its own thread with its own single-threaded runtime.
use std::cell::RefCell;
use std::ffi::{c_char, c_int, c_void, CStr, CString};
use std::rc::Rc;
use std::sync::Mutex;
use std::thread::JoinHandle;

use futures_channel::mpsc::{unbounded, UnboundedSender};

use crate::session::{self, Command, Event, Params, Sink};

#[repr(C)]
pub struct LrdpParams {
    pub username: *const c_char,
    pub password: *const c_char,
    pub domain: *const c_char,
    pub destination: *const c_char,
    pub proxy_auth: *const c_char,
    pub width: u16,
    pub height: u16,
}

#[repr(C)]
pub struct LrdpEvent {
    pub kind: i32,
    pub x: i32,
    pub y: i32,
    pub w: i32,
    pub h: i32,
    pub data: *const u8,
    pub len: usize,
    pub text: *const c_char,
}

type SendCb = extern "C" fn(*mut c_void, *const u8, usize);
type EventCb = extern "C" fn(*mut c_void, *const LrdpEvent);

#[derive(Clone, Copy)]
struct Raw {
    send: SendCb,
    event: EventCb,
    user: *mut c_void,
}
// the application promises that `user` may be used from the thread of the session
unsafe impl Send for Raw {}

pub struct LrdpSession {
    feed: Mutex<Option<UnboundedSender<Vec<u8>>>>,
    cmds: UnboundedSender<Command>,
    thread: Mutex<Option<JoinHandle<()>>>,
}

unsafe fn text(p: *const c_char) -> Option<String> {
    if p.is_null() { None } else { Some(CStr::from_ptr(p).to_string_lossy().into_owned()) }
}

#[no_mangle]
pub unsafe extern "C" fn lrdp_start(params: *const LrdpParams, send: SendCb, event: EventCb, user: *mut c_void) -> *mut LrdpSession {
    if params.is_null() { return std::ptr::null_mut(); }
    let p = &*params;
    let (Some(username), Some(password), Some(destination)) = (text(p.username), text(p.password), text(p.destination)) else { return std::ptr::null_mut() };
    if p.width == 0 || p.height == 0 { return std::ptr::null_mut(); }
    let params = Params {
        username,
        password,
        domain: text(p.domain).filter(|d| !d.is_empty()),
        destination,
        proxy_auth: text(p.proxy_auth).unwrap_or_default(),
        width: p.width,
        height: p.height,
    };
    let (feed_tx, feed_rx) = unbounded::<Vec<u8>>();
    let (cmd_tx, cmd_rx) = unbounded::<Command>();
    let raw = Raw { send, event, user };
    let thread = std::thread::Builder::new().name("linager-rdp".into()).spawn(move || {
        let raw = raw;
        let Ok(rt) = tokio::runtime::Builder::new_current_thread().enable_time().build() else { return };
        let send_fn: Rc<RefCell<dyn FnMut(&[u8])>> = Rc::new(RefCell::new(move |d: &[u8]| (raw.send)(raw.user, d.as_ptr(), d.len())));
        let event_fn = Box::new(move |e: Event<'_>| {
            let mut ev = LrdpEvent { kind: 0, x: 0, y: 0, w: 0, h: 0, data: std::ptr::null(), len: 0, text: std::ptr::null() };
            let keep: CString;
            match e {
                Event::Connected { width, height } => { ev.kind = 1; ev.w = i32::from(width); ev.h = i32::from(height); }
                Event::Frame { x, y, w, h, data } => {
                    ev.kind = 2; ev.x = i32::from(x); ev.y = i32::from(y); ev.w = i32::from(w); ev.h = i32::from(h);
                    ev.data = data.as_ptr(); ev.len = data.len();
                }
                Event::Ended(t) => {
                    ev.kind = 3;
                    keep = CString::new(t.replace('\0', " ")).unwrap_or_default();
                    ev.text = keep.as_ptr();
                    (raw.event)(raw.user, &ev);
                    return;
                }
            }
            (raw.event)(raw.user, &ev);
        });
        rt.block_on(session::run(params, feed_rx, cmd_rx, Sink { send: send_fn, event: event_fn }));
    });
    let Ok(thread) = thread else { return std::ptr::null_mut() };
    Box::into_raw(Box::new(LrdpSession { feed: Mutex::new(Some(feed_tx)), cmds: cmd_tx, thread: Mutex::new(Some(thread)) }))
}

#[no_mangle]
pub unsafe extern "C" fn lrdp_feed(s: *mut LrdpSession, data: *const u8, len: usize) {
    let Some(s) = s.as_ref() else { return };
    if data.is_null() { return; }
    if let Ok(g) = s.feed.lock() {
        if let Some(tx) = g.as_ref() { let _ = tx.unbounded_send(std::slice::from_raw_parts(data, len).to_vec()); }
    }
}

#[no_mangle]
pub unsafe extern "C" fn lrdp_feed_end(s: *mut LrdpSession) {
    let Some(s) = s.as_ref() else { return };
    if let Ok(mut g) = s.feed.lock() { *g = None; }
}

fn input(s: *mut LrdpSession, op: Option<ironrdp::input::Operation>) {
    let Some(s) = (unsafe { s.as_ref() }) else { return };
    if let Some(op) = op { let _ = s.cmds.unbounded_send(Command::Input(vec![op])); }
}

#[no_mangle]
pub extern "C" fn lrdp_mouse_move(s: *mut LrdpSession, x: u16, y: u16) { input(s, Some(session::op_move(x, y))) }

#[no_mangle]
pub extern "C" fn lrdp_mouse_button(s: *mut LrdpSession, button: c_int, down: c_int) { input(s, session::op_button(button, down != 0)) }

#[no_mangle]
pub extern "C" fn lrdp_wheel(s: *mut LrdpSession, dx: c_int, dy: c_int) {
    if dy != 0 { input(s, Some(session::op_wheel(true, dy))) }
    if dx != 0 { input(s, Some(session::op_wheel(false, dx))) }
}

#[no_mangle]
pub extern "C" fn lrdp_key(s: *mut LrdpSession, scancode: u8, extended: c_int, down: c_int) { input(s, Some(session::op_key(scancode, extended != 0, down != 0))) }

#[no_mangle]
pub extern "C" fn lrdp_unicode(s: *mut LrdpSession, scalar: u32, down: c_int) { input(s, session::op_unicode(scalar, down != 0)) }

#[no_mangle]
pub unsafe extern "C" fn lrdp_stop(s: *mut LrdpSession) {
    if s.is_null() { return; }
    let s = Box::from_raw(s);
    let _ = s.cmds.unbounded_send(Command::Close);
    if let Ok(mut g) = s.feed.lock() { *g = None; }
    let handle = s.thread.lock().ok().and_then(|mut t| t.take());
    if let Some(h) = handle { let _ = h.join(); }
}
