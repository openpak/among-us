// `dtls-front --selftest`: a Hazel DTLS client -> this front -> a plain Hazel UDP server, all on
// loopback. Passes when the server sees the client's handshake data and a reliable message, and
// the client gets the server's reply back through the DTLS session.

using System;
using System.Net;
using System.Security.Cryptography.X509Certificates;
using System.Threading;
using Hazel;
using Hazel.Dtls;
using Hazel.Udp;
using Hazel.Udp.FewerThreads;

namespace OpenPak.AmongUs.DtlsFront
{
    internal static class SelfTest
    {
        public static int Run()
        {
            var log = new ConsoleLogger();
            var upstreamEp = new IPEndPoint(IPAddress.Loopback, 42603);
            var frontEp = new IPEndPoint(IPAddress.Loopback, 42606);

            using var server = new ThreadLimitedUdpConnectionListener(2, upstreamEp, log);
            string gotHandshake = null, gotMessage = null, gotReply = null;
            var replied = new ManualResetEventSlim();
            server.NewConnection += e =>
            {
                gotHandshake = e.HandshakeData.ReadString();
                e.Connection.DataReceived += d =>
                {
                    gotMessage = d.Message.ReadString();
                    var w = MessageWriter.Get(SendOption.Reliable);
                    w.Write("pong:" + gotMessage);
                    d.Sender.Send(w);
                    w.Recycle();
                };
            };
            server.Start();

            using var front = new RelayListener(frontEp, upstreamEp, log);
            X509Certificate2 cert = Program.MakeCertificate();
            front.SetCertificate(cert);
            front.Start();

            using var client = new DtlsUnityConnection(log, frontEp);
            // Stands in for the console with the trust patch: the client accepts this front's certificate.
            client.SetValidServerCertificates(new X509Certificate2Collection(new X509Certificate2(cert.RawData)));
            client.DataReceived += d => { gotReply = d.Message.ReadString(); replied.Set(); };
            var hs = MessageWriter.Get(SendOption.None);
            hs.Write("hello-from-console");
            client.Connect(hs.ToByteArray(false), 5000);
            var m = MessageWriter.Get(SendOption.Reliable);
            m.Write("ping");
            client.Send(m);
            bool ok = replied.Wait(5000);
            Console.WriteLine($"[selftest] handshake={gotHandshake} message={gotMessage} reply={gotReply} relays={front.Count}");
            ok = ok && gotHandshake == "hello-from-console" && gotMessage == "ping" && gotReply == "pong:ping";
            Console.WriteLine(ok ? "[selftest] PASS" : "[selftest] FAIL");
            return ok ? 0 : 1;
        }
    }
}
