import base64
import json
import socket
import ssl
import sys
import time
from pathlib import Path


TARGET_HOST = "ipinfo.io"
TARGET_PORT = 443


def load_config(path: str) -> dict[str, str]:
    cfg: dict[str, str] = {}
    for raw in Path(path).read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        cfg[key.strip()] = value.strip()
    return cfg


def now() -> str:
    return time.strftime("%H:%M:%S")


def log(kind: str, message: str) -> None:
    print(f"{now()} {kind}: {message}", flush=True)


def recv_until(sock: socket.socket, marker: bytes, limit: int = 65536) -> bytes:
    data = bytearray()
    while marker not in data:
        chunk = sock.recv(4096)
        if not chunk:
            raise EOFError("connection closed before expected response")
        data.extend(chunk)
        if len(data) > limit:
            raise RuntimeError("response too large")
    return bytes(data)


def http_json_over_tls(sock: socket.socket) -> dict:
    ctx = ssl.create_default_context()
    with ctx.wrap_socket(sock, server_hostname=TARGET_HOST) as tls:
        req = (
            f"GET /json HTTP/1.1\r\n"
            f"Host: {TARGET_HOST}\r\n"
            "User-Agent: identity-keeper-probe/0.1\r\n"
            "Connection: close\r\n"
            "\r\n"
        ).encode()
        tls.sendall(req)
        data = bytearray()
        while True:
            chunk = tls.recv(8192)
            if not chunk:
                break
            data.extend(chunk)
    head, _, body = bytes(data).partition(b"\r\n\r\n")
    if b" 200 " not in head.split(b"\r\n", 1)[0]:
        raise RuntimeError(head.split(b"\r\n", 1)[0].decode("latin1", "replace"))
    return json.loads(body.decode("utf-8"))


def sleep_with_progress(kind: str, seconds: int) -> None:
    end = time.monotonic() + seconds
    while True:
        left = int(end - time.monotonic())
        if left <= 0:
            return
        step = min(30, left)
        log(kind, f"idle wait, {left}s remaining")
        time.sleep(step)


def http_idle_probe(cfg: dict[str, str], idle_seconds: int) -> None:
    kind = "HTTP-IDLE"
    host = cfg["PROXY_HOST"]
    port = int(cfg["PROXY_HTTP_PORT"])
    user = cfg["PROXY_USER"]
    password = cfg["PROXY_PASS"]
    auth = base64.b64encode(f"{user}:{password}".encode()).decode()

    log(kind, f"opening TCP connection to HTTP proxy, then idling {idle_seconds}s before CONNECT")
    sock = socket.create_connection((host, port), timeout=20)
    sock.settimeout(25)
    try:
        sleep_with_progress(kind, idle_seconds)
        connect = (
            f"CONNECT {TARGET_HOST}:{TARGET_PORT} HTTP/1.1\r\n"
            f"Host: {TARGET_HOST}:{TARGET_PORT}\r\n"
            f"Proxy-Authorization: Basic {auth}\r\n"
            "Proxy-Connection: Keep-Alive\r\n"
            "\r\n"
        ).encode()
        sock.sendall(connect)
        resp = recv_until(sock, b"\r\n\r\n")
        status = resp.split(b"\r\n", 1)[0].decode("latin1", "replace")
        if " 200 " not in status:
            raise RuntimeError(status)
        body = http_json_over_tls(sock)
        log(kind, f"PASS after idle; exit_ip={body.get('ip')} org={body.get('org')}")
    finally:
        sock.close()


def socks_idle_probe(cfg: dict[str, str], idle_seconds: int) -> None:
    kind = "SOCKS-IDLE"
    host = cfg["PROXY_HOST"]
    port = int(cfg["PROXY_SOCKS_PORT"])
    user = cfg["PROXY_USER"].encode()
    password = cfg["PROXY_PASS"].encode()

    log(kind, f"opening SOCKS5 connection and authenticating, then idling {idle_seconds}s before CONNECT")
    sock = socket.create_connection((host, port), timeout=20)
    sock.settimeout(25)
    try:
        sock.sendall(b"\x05\x01\x02")
        resp = sock.recv(2)
        log(kind, f"method response={resp!r}")
        if resp != b"\x05\x02":
            raise RuntimeError(f"unexpected SOCKS method response: {resp!r}")
        sock.sendall(bytes([1, len(user)]) + user + bytes([len(password)]) + password)
        resp = sock.recv(2)
        log(kind, f"auth response={resp!r}")
        if resp != b"\x01\x00":
            raise RuntimeError(f"SOCKS auth failed: {resp!r}")

        sleep_with_progress(kind, idle_seconds)
        host_bytes = TARGET_HOST.encode()
        req = b"\x05\x01\x00\x03" + bytes([len(host_bytes)]) + host_bytes + TARGET_PORT.to_bytes(2, "big")
        sock.sendall(req)
        log(kind, "sent CONNECT request")
        head = sock.recv(4)
        log(kind, f"connect response head={head!r}")
        if len(head) != 4 or head[1] != 0:
            raise RuntimeError(f"SOCKS connect failed: {head!r}")
        atyp = head[3]
        previous_timeout = sock.gettimeout()
        sock.settimeout(0.5)
        try:
            if atyp == 1:
                _ = sock.recv(4 + 2)
            elif atyp == 3:
                ln = sock.recv(1)[0]
                _ = sock.recv(ln + 2)
            elif atyp == 4:
                _ = sock.recv(16 + 2)
            else:
                raise RuntimeError(f"unexpected SOCKS atyp: {atyp}")
        except TimeoutError:
            log(kind, "SOCKS server omitted/delayed bind address; continuing after success status")
        finally:
            sock.settimeout(previous_timeout)
        body = http_json_over_tls(sock)
        log(kind, f"PASS after idle; exit_ip={body.get('ip')} org={body.get('org')}")
    finally:
        sock.close()


def main() -> int:
    cfg = load_config("proxy-secret.local")
    mode = sys.argv[1] if len(sys.argv) > 1 else "both"
    idle_seconds = int(sys.argv[2]) if len(sys.argv) > 2 else int(cfg.get("LONG_IDLE_SECONDS") or "360")
    try:
        if mode in ("http", "both"):
            http_idle_probe(cfg, idle_seconds)
        if mode in ("socks", "both"):
            socks_idle_probe(cfg, idle_seconds)
    except Exception as exc:
        log(mode.upper(), f"FAIL: {type(exc).__name__}: {exc}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
