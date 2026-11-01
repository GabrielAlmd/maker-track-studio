#!/usr/bin/env python3
"""Exercise the shipped Nginx configuration against a disposable standalone API.

Requires a built API binary and Nginx on Linux. No host trust-store changes,
real database, fixed ports, repository secrets or third-party Python packages.
"""
import argparse
import base64
import contextlib
import http.client
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def wait_for(check, processes, description):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if any(process.poll() is not None for process in processes):
            raise AssertionError(f"Process exited while waiting for {description}")
        try:
            if check():
                return
        except (OSError, ssl.SSLError, http.client.HTTPException):
            pass
        time.sleep(0.1)
    raise AssertionError(f"Timed out waiting for {description}")


@contextlib.contextmanager
def running(command, env, log):
    with log.open("ab") as output:
        process = subprocess.Popen(command, env=env, stdout=output, stderr=output)
        try:
            yield process
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()


def run(api, nginx, helper, web_root=None, browser=False):
    repository = Path(__file__).resolve().parents[1]
    with tempfile.TemporaryDirectory(prefix="maker-lan-test-") as temp:
        root = Path(temp)
        api_port, tls_port = free_port(), free_port()
        origin = f"https://localhost:{tls_port}"
        authority = f"localhost:{tls_port}"
        key = secrets.token_hex(32)
        password = secrets.token_urlsafe(24)
        rotated_password = secrets.token_urlsafe(24)
        key_file = root / "proxy.key"
        key_file.write_text(key)
        key_file.chmod(0o600)
        auth_file = root / "auth.htpasswd"
        web = root / "web"
        web.mkdir()
        if web_root:
            web = Path(web_root).resolve()
            assert (web / "index.html").is_file(), "Build the browser UI first"
        else:
            (web / "index.html").write_text("<!doctype html><title>LAN fixture</title>LAN fixture")

        env = {name: value for name, value in os.environ.items()
               if not name.startswith(("API_", "XDG_"))}
        env.update({
            "XDG_DATA_HOME": str(root / "data"),
            "XDG_CONFIG_HOME": str(root / "config"),
            "API_BIND_ADDRESS": "127.0.0.1", "API_PORT": str(api_port),
            "API_SQLITE_PATH": str(root / "test.db"),
            "API_DIAGNOSTICS_DIR": str(root / "logs"),
            "API_PUBLIC_ORIGIN": origin, "API_PROXY_SECRET_FILE": str(key_file),
            "API_TRUSTED_PROXIES": "127.0.0.1/32",
        })

        def write_password(value):
            # stdin avoids exposing the generated password in process arguments.
            hashed = subprocess.check_output(
                [helper, "hash-password"], input=value + "\n", text=True, env=env,
                stderr=subprocess.DEVNULL).strip()
            auth_file.write_text(hashed + "\n")
            auth_file.chmod(0o600)

        write_password(password)
        tls_dir = root / "tls"
        env["MAKER_TLS_DIR"] = str(tls_dir)
        # Prepare certificates through the same helper shipped in the image.
        subprocess.run([helper, "certificates"], env=env, check=True)
        config = (repository / "deployments/self-hosted/nginx.conf.template").read_text()
        for source, target in {
            "@@HOST@@": "localhost", "@@AUTHORITY@@": authority,
            "@@TLS@@": str(tls_dir), "@@KEY@@": key,
            "/etc/maker-track/auth.htpasswd": str(auth_file),
            "/tmp/nginx": str(root / "nginx"),
            "listen 8443": f"listen {tls_port}",
            "127.0.0.1:8080": f"127.0.0.1:{api_port}",
            "/srv/maker-track/web": str(web),
        }.items():
            config = config.replace(source, target)
        (root / "nginx").mkdir()
        config_file = root / "nginx.conf"
        config_file.write_text(config)
        subprocess.run([nginx, "-t", "-c", str(config_file)], env=env, check=True)

        # Disable ambient HTTP proxies; all tests must connect to local processes.
        tls_context = ssl.create_default_context()
        opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}),
            urllib.request.HTTPSHandler(context=tls_context))

        def request(path, expected, method="GET", credential=None, headers=None,
                    body=None, base=origin, raw_data=None):
            sent = dict(headers or {})
            if credential is not None:
                encoded = base64.b64encode(f"maker:{credential}".encode()).decode()
                sent["Authorization"] = "Basic " + encoded
            data = raw_data if body is None else json.dumps(body).encode()
            if body is not None:
                sent["Content-Type"] = "application/json"
            req = urllib.request.Request(base + path, data=data, headers=sent, method=method)
            try:
                response = opener.open(req, timeout=5)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                payload = response.read()
                assert response.status == expected, (
                    f"{method} {path}: expected {expected}, got {response.status}")
                return payload, response.headers

        with running([api], env, root / "api.log") as backend:
            def backend_ready():
                request("/health/live", 401, base=f"http://127.0.0.1:{api_port}")
                return True

            wait_for(backend_ready, [backend], "API")
            with running([nginx, "-c", str(config_file), "-g", "daemon off;"],
                         env, root / "nginx.log") as proxy:
                ca = tls_dir / "root.crt"
                wait_for(ca.exists, [backend, proxy], "private test CA")
                tls_context.load_verify_locations(cafile=str(ca))

                def proxy_ready():
                    request("/", 401)
                    return True

                wait_for(proxy_ready, [backend, proxy], "verified HTTPS proxy")
                # A fresh client without the private CA must reject the certificate.
                untrusted = http.client.HTTPSConnection(
                    "localhost", tls_port, context=ssl.create_default_context())
                try:
                    try:
                        untrusted.request("GET", "/")
                        untrusted.getresponse()
                    except ssl.SSLCertVerificationError:
                        pass
                    else:
                        raise AssertionError("Untrusted private CA was accepted")
                finally:
                    untrusted.close()

                sensitive = ["/", "/api/health/live", "/api/health/ready",
                             "/api/printers", "/api/desktop-data/export",
                             "/api/reports/export", "/api/desktop-data/import",
                             "/api/data-migration/import", "/api/sales-orders",
                             "/api/unknown"]
                for path in sensitive:
                    request(path, 401)
                    request(path, 401, credential="invalid")
                    request(path, 401, headers={"X-Maker-Proxy-Key": key})
                for method, path in [
                    ("POST", "/api/desktop-data/import"),
                    ("POST", "/api/data-migration/import"),
                    ("DELETE", "/api/models/00000000-0000-0000-0000-000000000000"),
                ]:
                    request(path, 401, method=method, headers={"Origin": origin})
                    request(path, 403, method=method, credential=password)
                    request(path, 403, method=method, credential=password,
                            headers={"Origin": "https://attacker.invalid"})

                page, _ = request("/", 200, credential=password)
                assert b"<!doctype html>" in page.lower()
                request("/deep/spa/path", 200, credential=password)
                request("/api/health/ready", 200, credential=password)
                # Client-supplied proxy/forwarding headers are overwritten.
                request("/api/printers", 200, credential=password, headers={
                    "X-Maker-Proxy-Key": "forged", "X-Forwarded-Proto": "http",
                    "X-Forwarded-Host": "attacker.invalid"})
                request("/api/printers", 403, credential=password,
                        headers={"Origin": "null"})
                request("/api/printers", 403, credential=password,
                        headers={"Sec-Fetch-Site": "cross-site"})
                payload, headers = request(
                    "/api/printers", 201, method="POST", credential=password,
                    headers={"Origin": origin},
                    body={"name": "Proxy fixture", "manufacturer": "Test", "model": "Fixture"})
                printer = json.loads(payload)
                request("/api/printers/" + printer["id"], 200, credential=password)
                assert headers["Cache-Control"] == "no-store"
                assert headers["X-Content-Type-Options"] == "nosniff"
                backup, _ = request("/api/desktop-data/export", 200, credential=password)
                assert backup.startswith(b"PK"), "Backup should be a ZIP download"
                boundary = "maker-" + secrets.token_hex(16)
                multipart = (
                    f"--{boundary}\r\n"
                    'Content-Disposition: form-data; name="file"; filename="fixture.zip"\r\n'
                    "Content-Type: application/zip\r\n\r\n"
                ).encode() + backup + f"\r\n--{boundary}--\r\n".encode()
                request("/api/desktop-data/import", 200, method="POST",
                        credential=password, headers={
                            "Origin": origin,
                            "Content-Type": f"multipart/form-data; boundary={boundary}"},
                        raw_data=multipart)
                request("/api/printers/" + printer["id"], 200, credential=password)


                # Direct API calls still fail with a forged proxy identity.
                request("/printers", 401, base=f"http://127.0.0.1:{api_port}",
                        headers={"Host": authority, "X-Maker-Proxy-Key": "forged",
                                 "X-Forwarded-Proto": "https", "Origin": origin})

                # DNS rebinding/forged Host must not return UI or application data.
                conn = http.client.HTTPSConnection("localhost", tls_port, context=tls_context)
                try:
                    auth = base64.b64encode(f"maker:{password}".encode()).decode()
                    conn.request("GET", "/api/printers", headers={
                        "Host": "attacker.invalid", "Authorization": "Basic " + auth})
                    response = conn.getresponse()
                    response.read()
                    assert response.status == 421, response.status
                finally:
                    conn.close()

                if browser:
                    assert web_root, "--browser requires --web-root with the production build"
                    with socket.create_connection(("localhost", tls_port)) as raw:
                        with tls_context.wrap_socket(raw, server_hostname="localhost") as tls:
                            certificate = tls.getpeercert(binary_form=True)
                    public_key = subprocess.check_output(
                        ["openssl", "x509", "-inform", "DER", "-pubkey", "-noout"],
                        input=certificate)
                    der_key = subprocess.check_output(
                        ["openssl", "pkey", "-pubin", "-outform", "DER"], input=public_key)
                    pin = base64.b64encode(hashlib.sha256(der_key).digest()).decode()
                    subprocess.run(
                        ["node", str(repository / "apps/web/integration/lan-proxy.mjs")],
                        input=json.dumps({"origin": origin, "password": password,
                                          "certificatePin": pin}),
                        text=True, check=True, timeout=60)

                write_password(rotated_password)
                subprocess.run([nginx, "-c", str(config_file), "-s", "reload"],
                               env=env, check=True)
                time.sleep(0.3)
                request("/api/printers", 401, credential=password)
                request("/api/printers", 200, credential=rotated_password)
                print("PASS: verified TLS, proxy authentication/routing, CSRF, forged headers, "
                      "direct API denial, CRUD, backup export/restore and password rotation")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api", required=True, help="Absolute path to the built standalone API")
    parser.add_argument("--nginx", default="nginx", help="Nginx executable")
    parser.add_argument("--helper", required=True, help="Built selfhostctl executable")
    parser.add_argument("--web-root", help="Production browser build directory (otherwise a fixture)")
    parser.add_argument("--browser", action="store_true", help="Also exercise the real UI in Chromium")
    args = parser.parse_args()
    run(args.api, args.nginx, args.helper, args.web_root, args.browser)
