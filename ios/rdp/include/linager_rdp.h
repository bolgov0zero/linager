// The C interface of the RDP core (Rust, IronRDP). The core knows no sockets: the application moves the messages of the WebSocket
// (RDCleanPath of the Linager server) in with lrdp_feed and out through the send callback. Callbacks come from the thread of the session.
#ifndef LINAGER_RDP_H
#define LINAGER_RDP_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct lrdp_session lrdp_session;

typedef struct {
    const char *username;
    const char *password;
    const char *domain;      // may be NULL
    const char *destination; // "address:port" of the computer as the server knows it
    const char *proxy_auth;  // the token for the RDCleanPath request
    uint16_t width;
    uint16_t height;
} lrdp_params;

enum {
    LRDP_CONNECTED = 1, // w, h: the size of the desktop
    LRDP_FRAME = 2,     // x, y, w, h: a changed rectangle; data: w*h*4 bytes RGBA, rows packed
    LRDP_ENDED = 3,     // text: why (an error text, or "" for a plain end); the session is over
};

typedef struct {
    int32_t kind;
    int32_t x, y, w, h;
    const uint8_t *data;
    size_t len;
    const char *text;
} lrdp_event;

// One message for the WebSocket.
typedef void (*lrdp_send_cb)(void *user, const uint8_t *data, size_t len);
typedef void (*lrdp_event_cb)(void *user, const lrdp_event *event);

// Starts a session on its own thread. Returns NULL when the parameters are not usable.
lrdp_session *lrdp_start(const lrdp_params *params, lrdp_send_cb send, lrdp_event_cb event, void *user);
// A message from the WebSocket.
void lrdp_feed(lrdp_session *s, const uint8_t *data, size_t len);
// The WebSocket is closed (nothing more will come).
void lrdp_feed_end(lrdp_session *s);

// Input. Coordinates are pixels of the desktop. Buttons: 0 left, 1 middle, 2 right, 3 back, 4 forward.
void lrdp_mouse_move(lrdp_session *s, uint16_t x, uint16_t y);
void lrdp_mouse_button(lrdp_session *s, int button, int down);
// Wheel rotation in notches (120 units each); positive is down and to the right.
void lrdp_wheel(lrdp_session *s, int dx, int dy);
// A key by its scan code (PC set 1); `extended` for the E0 keys.
void lrdp_key(lrdp_session *s, uint8_t scancode, int extended, int down);
// A character by its Unicode scalar value, independent of the layout of the computer.
void lrdp_unicode(lrdp_session *s, uint32_t scalar, int down);

// Ends the session and frees it. The session may not be used after this. Blocks until its thread has ended (it is quick).
void lrdp_stop(lrdp_session *s);

#ifdef __cplusplus
}
#endif

#endif
