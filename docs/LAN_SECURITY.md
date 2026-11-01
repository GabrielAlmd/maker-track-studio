# LAN security boundary

Nginx serves HTTPS on container port 8443 and proxies to the loopback API.
Use the [self-hosted installation guide](SELF_HOSTED.md) for manual deployment,
certificate trust, firewall rules, storage and recovery.

All UI and API paths require the shared maker login, using bcrypt and HTTPS.
The proxy removes Authorization before forwarding, replaces proxy identity
headers and rejects unknown Host values. The API checks origin and proxy secret.
Never expose port 8080 or publish this service to WAN/guest networks.

Generate proxy.key and auth.htpasswd with setup-secrets.sh using the selected
release image. Protect secrets outside Git and make them readable only by the
installation owner and container UID 10001. reset-password.sh atomically replaces
the hash and recreates the container. Verify the old password fails. Browsers may
cache credentials until closed. Rotate proxy.key while stopped, then recreate
the service so both API and proxy load the same secret.

The TLS helper persists a private local CA and automatically renews server
certificates. Install only root.crt in client trust stores, after verifying its
fingerprint on the server. Never distribute root.key. See the installation guide
for CA replacement and backup. Public ACME and router port forwarding are not used.

CI exercises verified TLS, authentication, forged headers, CSRF, direct API
denial, SPA routing, CRUD, backup export/import and password rotation using
throwaway SQLite data. Native image scans block HIGH/CRITICAL vulnerabilities.

References: https://nginx.org/en/docs/http/ngx_http_auth_basic_module.html
and https://nginx.org/en/docs/control.html.
