// DTLS front for Impostor (OpenPak among-us).
//
// The console client reaches the game server with Hazel DTLS on Impostor's port + 3. Impostor
// speaks plain Hazel UDP. This program terminates the DTLS session with Hazel's own server-side
// listener and, per player, opens a plain Hazel connection to Impostor with the same handshake
// data, then copies every message both ways with its send option, unchanged. Disconnects (and
// their reason payload) pass through.
//
// The server certificate is OpenPak's fixed DTLS certificate (AMONG_US_DTLS_CERT and
// AMONG_US_DTLS_KEY, PEM files on the box). The console accepts it with the OpenPak DTLS trust
// patch (tools/dtls-trust). With neither set, a throwaway certificate is generated for local tests.
//
//   AMONG_US_DTLS_CERT      PEM certificate (default unset)
//   AMONG_US_DTLS_KEY       PEM private key, RSA (default unset)
//   AMONG_US_DTLS_ADDR      listen address (default 0.0.0.0:22606)
//   AMONG_US_DTLS_UPSTREAM  Impostor's game endpoint (default 127.0.0.1:22603)

using System;
using System.Collections.Concurrent;
using System.Net;
using System.Net.Sockets;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using System.Threading;
using System.Threading.Tasks;
using Hazel;
using Hazel.Dtls;
using Hazel.Udp;

namespace OpenPak.AmongUs.DtlsFront
{
    internal sealed class ConsoleLogger : ILogger
    {
        public void WriteVerbose(string msg) { }
        public void WriteError(string msg) => Log("error", msg);
        public void WriteWarning(string msg) => Log("warn", msg);
        public void WriteInfo(string msg) => Log("info", msg);
        // Hazel's messages name endpoints; client addresses are personal data, so they are cut.
        private static void Log(string level, string msg)
        {
            int at = msg.IndexOf(" from ", StringComparison.Ordinal);
            if (at >= 0) msg = msg.Substring(0, at);
            Console.WriteLine($"[dtls] {level}: {msg}");
        }
    }

    // One player: the DTLS-side connection the console opened and a plain Hazel UDP connection to
    // Impostor opened with the same handshake data. Messages are copied across with their send
    // option, so reliability is per hop and the payload is unchanged.
    internal sealed class Session
    {
        private readonly Connection client;
        private readonly UnityUdpClientConnection upstream;
        private readonly object gate = new object();
        private System.Collections.Generic.List<(byte[], SendOption)> pending = new System.Collections.Generic.List<(byte[], SendOption)>();
        private int closed;
        private long up, down;
        private readonly long opened = Environment.TickCount64;

        public Session(Connection client, IPEndPoint impostor, byte[] handshake, ILogger logger, Action onClose)
        {
            this.client = client;
            this.upstream = new UnityUdpClientConnection(logger, impostor);
            client.DataReceived += e => Forward(e, toImpostor: true);
            this.upstream.DataReceived += e => Forward(e, toImpostor: false);
            client.Disconnected += (_, e) => { if (Close(this.upstream, e)) onClose(); };
            this.upstream.Disconnected += (_, e) => { if (Close(this.client, e)) onClose(); };
            Task.Run(() =>
            {
                try
                {
                    this.upstream.Connect(handshake, 5000);
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"[dtls] error: Impostor did not accept the connection: {ex.GetType().Name}");
                    if (System.Threading.Interlocked.Exchange(ref this.closed, 1) == 0)
                    {
                        this.client.Disconnect("upstream unavailable");
                        onClose();
                    }
                    return;
                }
                System.Collections.Generic.List<(byte[], SendOption)> queued;
                lock (this.gate)
                {
                    queued = this.pending;
                    this.pending = null;
                }
                foreach (var (bytes, option) in queued) Send(this.upstream, bytes, option);
            });
        }

        private void Forward(DataReceivedEventArgs e, bool toImpostor)
        {
            try
            {
                if (toImpostor) System.Threading.Interlocked.Increment(ref this.up);
                else System.Threading.Interlocked.Increment(ref this.down);
                byte[] bytes = new byte[e.Message.Length];
                Buffer.BlockCopy(e.Message.Buffer, e.Message.Offset, bytes, 0, bytes.Length);
                if (toImpostor)
                {
                    lock (this.gate)
                    {
                        if (this.pending != null)
                        {
                            this.pending.Add((bytes, e.SendOption));
                            return;
                        }
                    }
                    Send(this.upstream, bytes, e.SendOption);
                }
                else
                {
                    Send(this.client, bytes, e.SendOption);
                }
            }
            finally
            {
                e.Message.Recycle();
            }
        }

        private static void Send(Connection to, byte[] bytes, SendOption option)
        {
            if (to.State != ConnectionState.Connected) return;
            MessageWriter w = MessageWriter.Get(option);
            w.Write(bytes);
            to.Send(w);
            w.Recycle();
        }

        // A disconnect on one side closes the other with the same reason payload (Impostor's
        // disconnect reasons reach the console unchanged).
        private bool Close(Connection other, DisconnectedEventArgs e)
        {
            if (System.Threading.Interlocked.Exchange(ref this.closed, 1) == 1) return false;
            Console.WriteLine($"[dtls] session end: {(other == this.upstream ? "console" : "Impostor")} closed after " +
                $"{(Environment.TickCount64 - this.opened) / 1000.0:F1}s, {this.up} messages up, {this.down} down");
            MessageWriter w = null;
            if (e.Message != null && e.Message.Length > 0)
            {
                w = MessageWriter.Get(SendOption.None);
                w.Write(e.Message.Buffer, e.Message.Offset, e.Message.Length);
            }
            try { other.Disconnect(e.Reason ?? "relay", w); } catch (Exception) { }
            w?.Recycle();
            return true;
        }
    }

    internal sealed class RelayListener : IDisposable
    {
        private readonly DtlsConnectionListener listener;
        private int count;

        public RelayListener(IPEndPoint listen, IPEndPoint impostor, ILogger logger)
        {
            this.listener = new DtlsConnectionListener(2, listen, logger);
            this.listener.NewConnection += e =>
            {
                byte[] handshake = new byte[e.HandshakeData.Length];
                Buffer.BlockCopy(e.HandshakeData.Buffer, e.HandshakeData.Offset, handshake, 0, handshake.Length);
                int n = System.Threading.Interlocked.Increment(ref this.count);
                Console.WriteLine($"[dtls] session open ({n} active)");
                _ = new Session(e.Connection, impostor, handshake, logger, () =>
                {
                    int left = System.Threading.Interlocked.Decrement(ref this.count);
                    Console.WriteLine($"[dtls] session closed ({left} active)");
                });
            };
        }

        public int Count => this.count;
        public void SetCertificate(X509Certificate2 cert) => this.listener.SetCertificate(cert);
        public void Start() => this.listener.Start();
        public void Dispose() => this.listener.Dispose();
    }

    internal static class Program
    {
        private static IPEndPoint Parse(string value, string fallback)
        {
            string s = string.IsNullOrEmpty(value) ? fallback : value;
            return IPEndPoint.Parse(s);
        }

        internal static X509Certificate2 MakeCertificate()
        {
            using RSA rsa = RSA.Create(2048);
            var req = new CertificateRequest("CN=OpenPak Among Us DTLS", rsa, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
            using X509Certificate2 cert = req.CreateSelfSigned(DateTimeOffset.UtcNow.AddDays(-1), DateTimeOffset.UtcNow.AddYears(20));
            // Round-trip through PKCS#12 so the private key is usable on every platform.
            return new X509Certificate2(cert.Export(X509ContentType.Pkcs12), (string)null, X509KeyStorageFlags.Exportable);
        }

        private static int Main(string[] args)
        {
            if (args.Length > 0 && args[0] == "--selftest") return SelfTest.Run();
            if (args.Length > 1 && args[0] == "--probe") return SelfTest.Probe(IPEndPoint.Parse(args[1]));
            IPEndPoint listen = Parse(Environment.GetEnvironmentVariable("AMONG_US_DTLS_ADDR"), "0.0.0.0:22606");
            IPEndPoint upstream = Parse(Environment.GetEnvironmentVariable("AMONG_US_DTLS_UPSTREAM"), "127.0.0.1:22603");

            var listener = new RelayListener(listen, upstream, new ConsoleLogger());
            string certPath = Environment.GetEnvironmentVariable("AMONG_US_DTLS_CERT");
            string keyPath = Environment.GetEnvironmentVariable("AMONG_US_DTLS_KEY");
            X509Certificate2 cert;
            if (!string.IsNullOrEmpty(certPath))
            {
                using X509Certificate2 pem = X509Certificate2.CreateFromPemFile(certPath, keyPath);
                cert = new X509Certificate2(pem.Export(X509ContentType.Pkcs12), (string)null, X509KeyStorageFlags.Exportable);
                Console.WriteLine($"[dtls] certificate {cert.GetCertHashString(HashAlgorithmName.SHA256)}");
            }
            else
            {
                cert = MakeCertificate();
                Console.WriteLine("[dtls] warn: no AMONG_US_DTLS_CERT, using a throwaway certificate (tests only)");
            }
            listener.SetCertificate(cert);
            listener.Start();
            Console.WriteLine($"[dtls] listening on {listen}, relaying to {upstream}");

            var done = new ManualResetEventSlim();
            AppDomain.CurrentDomain.ProcessExit += (_, _) => done.Set();
            done.Wait();
            listener.Dispose();
            return 0;
        }
    }
}
