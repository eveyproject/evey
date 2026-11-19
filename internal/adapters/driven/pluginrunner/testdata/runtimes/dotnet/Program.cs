using System.Net.Sockets;

// Binds the plugin socket, then reports in started.txt (its install dir).
var sock = Environment.GetEnvironmentVariable("EVEY_PLUGIN_SOCK")!;
var s = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
s.Bind(new UnixDomainSocketEndPoint(sock));
s.Listen(1);
var seccompLib = File.ReadAllText("/proc/self/maps").Contains("libevey-sandbox.so") ? "true" : "false";
File.WriteAllText("started.txt",
    $"dotnet {Environment.Version} seccomp_lib={seccompLib} max_heap={GC.GetGCMemoryInfo().TotalAvailableMemoryBytes}\n");
Thread.Sleep(600_000);
