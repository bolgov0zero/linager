//! The RDP core of Linager for iOS: IronRDP over the RDCleanPath proxy of the Linager server. The core knows no sockets and no TLS: the
//! server upgrades the connection to the computer, the application moves the messages of its own WebSocket in and out (see `ffi`).
mod session;
pub mod ffi;

pub use session::{run, Command, Event, Params, Sink};
