# Evidence: among-us

**Blocker (2026-10-24): the game server connection is DTLS with a pinned certificate.** After the
matchmaker names the server, every online path of the console client opens a Hazel DTLS 1.2
session (game port + 3, or an auth handshake on port + 2), and it accepts only one compiled-in,
self-signed server certificate. No server without that certificate's private key can complete
the handshake, and DNS, the console CA store or the signed ceiling cannot change it. Going further
needs a console-side change to the client's certificate check plus a DTLS front for Impostor;
that is an owner decision under the no-game-patches rule. The same client code ships on the
other consoles.
