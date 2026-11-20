import java.net.StandardProtocolFamily;
import java.net.UnixDomainSocketAddress;
import java.nio.channels.ServerSocketChannel;
import java.nio.file.Files;
import java.nio.file.Path;

// Binds the plugin socket, then reports in started.txt (its install dir).
public class Plugin {
    public static void main(String[] args) throws Exception {
        String sock = System.getenv("EVEY_PLUGIN_SOCK");
        ServerSocketChannel ch = ServerSocketChannel.open(StandardProtocolFamily.UNIX);
        ch.bind(UnixDomainSocketAddress.of(sock));
        boolean seccompLib = Files.readString(Path.of("/proc/self/maps")).contains("libevey-sandbox.so");
        Files.writeString(Path.of("started.txt"),
            "java " + Runtime.version() + " seccomp_lib=" + seccompLib
                + " max_heap=" + Runtime.getRuntime().maxMemory() + "\n");
        Thread.sleep(600_000);
    }
}
