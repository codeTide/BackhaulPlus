# BackhaulPlus

Welcome to the **`BackhaulPlus`** project! This project is an enhanced fork of Backhaul, providing a **multi-server high-performance reverse tunneling solution**, optimized for handling massive concurrent connections through NAT and firewalls. This README will guide you through setting up and configuring both server and client components, including details on different transport protocols.

---

## Table of Contents

1. [Introduction](#introduction)
2. [Features](#features)
3. [Installation](#installation)
4. [Usage](#usage)
   - [Configuration Options](#configuration-options)
   - [Detailed Configuration](#detailed-configuration)
      - [TCP Configuration](#tcp-configuration)
      - [TCP Multiplexing Configuration](#tcp-multiplexing-configuration)
      - [UDP Configuration](#udp-configuration)
      - [WebSocket Configuration](#websocket-configuration)
      - [Secure WebSocket Configuration](#secure-websocket-configuration)
      - [WS Multiplexing Configuration](#ws-multiplexing-configuration)
      - [WSS Multiplexing Configuration](#wss-multiplexing-configuration)
5. [Raw Ports vs. SNI-based Routing](#raw-ports-vs-sni-based-routing)
6. [Generating a Self-Signed TLS Certificate with OpenSSL](#generating-a-self-signed-tls-certificate-with-openssl)
7. [Running BackhaulPlus as a service](#running-backhaulplus-as-a-service)
8. [FAQ](#faq)
9. [Benchmark](#benchmark)
10. [License](#license)
11. [Donation](#donation)

---

## Introduction

This project offers a robust **multi-server reverse tunneling solution** to overcome NAT and firewall restrictions, supporting various transport protocols. It’s engineered for **high efficiency and concurrency**, plus it adds the ability to run multiple independent server instances from a single configuration.

## Features

* **Multi-Server Ready**: Easily define and run multiple independent server instances from a single config file.
* **High Performance**: Optimized for handling massive concurrent connections efficiently.
* **Protocol Flexibility**: Supports TCP, WebSocket (WS), Secure WebSocket (WSS) and more.
* **UDP over TCP**: Implements UDP traffic encapsulation and forwarding over a TCP connection for reliable delivery with built-in congestion control.
* **Multiplexing**: Enables multiple connections over a single transport with SMUX.
* **NAT & Firewall Bypass**: Overcomes restrictions with reverse tunneling.
* **Traffic Sniffing**: Optional network traffic monitoring with logging support.
* **Configurable Keepalive**: Adjustable keep-alive and heartbeat intervals for stable connections.
* **TLS Encryption**: Secure connections via WSS with support for custom TLS certificates.
* **Web Interface**: Real-time monitoring through a lightweight web interface.
* **Hot Reload Configuration**: Supports dynamic configuration reloading without server restarts.

## Installation

1. **Download** the latest release from the [GitHub releases page](https://github.com/codeTide/BackhaulPlus/releases).
2. **Extract** the archive (adjust the `filename` if needed):

   ```bash
   tar -xzf backhaulplus_linux_amd64.tar.gz
   ``` 
3. **Run** the executable:  

   ```bash
   ./BackhaulPlus
   ```
4. You can also build from source if preferred:  

   ```bash
   git clone https://github.com/codeTide/BackhaulPlus.git
   cd BackhaulPlus
   go build
   ./BackhaulPlus
   ```

## Usage

The main executable for this project is `BackhaulPlus`. It requires a TOML configuration file for both the server and client components.

### Configuration Options

To start using the solution, you'll need to configure both server and client components. Here’s how to set up basic configurations:

* **Server Configuration**

   Create a configuration file named `config.toml`:

    ```toml
    [[server]]
    name = "SRV1"                 # Custom name for this server instance, used as prefix in logs (optional).
    bind_addr = "0.0.0.0:3080"    # Address and port for the server to listen on (mandatory).
    transport = "tcp"             # Protocol to use ("tcp", "tcpmux", "ws", "wss", "wsmux", "wssmux". mandatory).
    accept_udp = false            # Enable transferring UDP connections over TCP transport. (optional, default: false)
    token = "your_token"          # Authentication token for secure communication (optional).
    allow_multi_ip = false        # Allow tunnel/control connections from multiple client IPs on one server instance. (optional, default: false)
    keepalive_period = 75         # Interval in seconds to send keep-alive packets.(optional, default: 75s)
    nodelay = false               # Enable TCP_NODELAY (optional, default: false).
    channel_size = 2048           # Tunnel and Local channel size. Excess connections are discarded. (optional, default: 2048).
    heartbeat = 40                # In seconds. Ping interval for tunnel stability. Min: 1s. (Optional, default: 40s)
    mux_con = 8                   # Mux concurrency. Number of connections that can be multiplexed into a single stream (optional, default: 8).
    mux_version = 1               # SMUX protocol version (1 or 2). Version 2 may have extra features. (optional)
    mux_framesize = 32768         # 32 KB. The maximum size of a frame that can be sent over a connection. (optional)
    mux_recievebuffer = 4194304   # 4 MB. The maximum buffer size for incoming data per connection. (optional)
    mux_streambuffer = 65536      # 256 KB. The maximum buffer size per individual stream within a connection. (optional)
    sniffer = false               # Enable or disable network sniffing for monitoring data. (optional, default false)
    web_port = 2060               # Port number for the web interface or monitoring interface. (optional, set to 0 to disable).
    sniffer_log = "/root/log.json" # Filename used to store network traffic and usage data logs. (optional, default backhaul.json)
    tls_cert = "/root/server.crt" # Path to the TLS certificate file for wss/wssmux. (mandatory for wss/wssmux).
    tls_key = "/root/server.key"  # Path to the TLS private key file for wss/wssmux. (mandatory for wss/wssmux).
    log_level = "info"            # Log level ("panic", "fatal", "error", "warn", "info", "debug", "trace", optional, default: "info").

    # NOTE: the old `ports` field has been removed. Use `raw_ports` instead.
    raw_ports = [
      "443-600",                  # Listen on all ports in the range 443 to 600
      "443-600:5201",             # Listen on all ports in the range 443 to 600 and forward traffic to 5201
      "443-600=1.1.1.1:5201",     # Listen on all ports in the range 443 to 600 and forward traffic to 1.1.1.1:5201
      "443",                      # Listen on local port 443 and forward to remote port 443 (default forwarding).
      "4000=5000",                # Listen on local port 4000 (bind to all local IPs) and forward to remote port 5000.
      "127.0.0.2:443=5201",       # Bind to specific local IP (127.0.0.2), listen on port 443, and forward to remote port 5201.
      "443=1.1.1.1:5201",         # Listen on local port 443 and forward to a specific remote IP (1.1.1.1) on port 5201.
      "127.0.0.2:443=1.1.1.1:5201" # Bind to specific local IP (127.0.0.2), listen on port 443, and forward to remote IP (1.1.1.1) on port 5201.
    ]

    # SNI-based internal TCP routing (optional). See the "SNI-based Routing"
    # section below for details.
    sni_router = false
    sni_listen_addr = "0.0.0.0:443"
    sni_inspect_timeout = 1
    sni_default_action = "reject"
    sni_routes = [
      { sni = "myket.ir", target = "10001" },
      { sni = "cafebazaar.ir", target = "10002" }
    ]
    ```

   To start the `server`:

   ```sh
   ./backhaulplus -c config.toml
   ```
* **Client Configuration**

   Create a configuration file named `config.toml` for the client:
   ```toml
   [[client]]  # Behind NAT, firewall-blocked
   name = "DEClient"             # Custom name for this client instance, used as prefix in logs (optional).
   remote_addr = "0.0.0.0:3080"  # Server address and port (mandatory).
   edge_ip = "188.114.96.0"      # Edge IP used for CDN connection, specifically for WebSocket-based transports.(Optional, default none)
   transport = "tcp"             # Protocol to use ("tcp", "tcpmux", "ws", "wss", "wsmux", "wssmux". mandatory).
   token = "your_token"          # Authentication token for secure communication (optional).
   connection_pool = 8           # Number of pre-established connections.(optional, default: 8).
   aggressive_pool = false       # Enables aggressive connection pool management.(optional, default: false).
   keepalive_period = 75         # Interval in seconds to send keep-alive packets. (optional, default: 75s)
   nodelay = false               # Use TCP_NODELAY (optional, default: false).
   retry_interval = 3            # Retry interval in seconds (optional, default: 3s).
   dial_timeout = 10             # Sets the max wait time for establishing a network connection. (optional, default: 10s)
   mux_version = 1               # SMUX protocol version (1 or 2). Version 2 may have extra features. (optional)
   mux_framesize = 32768         # 32 KB. The maximum size of a frame that can be sent over a connection. (optional)
   mux_recievebuffer = 4194304   # 4 MB. The maximum buffer size for incoming data per connection. (optional)
   mux_streambuffer = 65536      # 256 KB. The maximum buffer size per individual stream within a connection. (optional)
   sniffer = false               # Enable or disable network sniffing for monitoring data. (optional, default false)
   web_port = 2060               # Port number for the web interface or monitoring interface. (optional, set to 0 to disable).
   sniffer_log ="/root/log.json" # Filename used to store network traffic and usage data logs. (optional, default backhaul.json)
   log_level = "info"            # Log level ("panic", "fatal", "error", "warn", "info", "debug", "trace", optional, default: "info").
   ```

   You can define multiple `[[client]]` blocks in the same `config.toml` to connect one BackhaulPlus process to multiple different servers simultaneously.

   To start the `client`:

   ```sh
   ./backhaul -c config.toml
   ```

### Detailed Configuration
#### TCP Configuration
* **Server**:

   ```toml
   [[server]]
   bind_addr = "0.0.0.0:3080"
   transport = "tcp"
   accept_udp = false 
   token = "your_token"
   allow_multi_ip = false 
   keepalive_period = 75  
   nodelay = true 
   heartbeat = 40 
   channel_size = 2048
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   raw_ports = [
     "8080"
   ]
   ```
* **Client**:

   ```toml
   [[client]]
   remote_addr = "0.0.0.0:3080"
   transport = "tcp"
   token = "your_token" 
   connection_pool = 8
   aggressive_pool = false
   keepalive_period = 75
   dial_timeout = 10
   nodelay = true 
   retry_interval = 3
   sniffer = false
   web_port = 2060 
   sniffer_log = "/root/backhaul.json"
   log_level = "info"

   ```
* **Details**:

   `remote_addr`: The IPv4, IPv6, or domain address of the server to which the client connects.

   `token`: An authentication token used to securely validate and authenticate the connection between the client and server within the tunnel.

   `channel_size`: The queue size for forwarding packets from server to the client. If the limit is exceeded, packets will be dropped.

   `connection_pool`: Set the number of pre-established connections for better latency.
   
   `nodelay`: Refers to a TCP socket option (TCP_NODELAY) that improve the latency but decrease the bandwidth


#### TCP Multiplexing Configuration
* **Server**:

   ```toml
   [[server]]
   bind_addr = "0.0.0.0:3080"
   transport = "tcpmux"
   token = "your_token" 
   allow_multi_ip = false 
   keepalive_period = 75
   nodelay = true 
   heartbeat = 40 
   channel_size = 2048
   mux_con = 8
   mux_version = 1
   mux_framesize = 32768 
   mux_recievebuffer = 4194304
   mux_streambuffer = 65536 
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   raw_ports = [
     "8080"
   ]
   ```
* **Client**:

   ```toml
   [[client]]
   remote_addr = "0.0.0.0:3080"
   transport = "tcpmux"
   token = "your_token" 
   connection_pool = 8
   aggressive_pool = false
   keepalive_period = 75
   dial_timeout = 10
   retry_interval = 3
   nodelay = true 
   mux_version = 1
   mux_framesize = 32768 
   mux_recievebuffer = 4194304
   mux_streambuffer = 65536 
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   ```
* **Details**:

   `mux_session`: Number of multiplexed sessions. Increase this if you need to handle more simultaneous sessions over a single connection.
   
   * `allow_multi_ip`: When set to `true` on server transports that use a control channel (e.g. `tcp`, `tcpmux`, `quic`), the server accepts tunnel/control connections from different source IPs instead of pinning to the first client IP.

   * Refer to TCP configuration for more information.


#### UDP Configuration
* **Server**:

   ```toml
   [[server]]
   bind_addr = "0.0.0.0:3080"
   transport = "udp"
   token = "your_token"
   heartbeat = 20 
   channel_size = 2048
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   raw_ports = [
     "8080"
   ]
   ```
* **Client**:

   ```toml
   [[client]]
   remote_addr = "0.0.0.0:3080"
   transport = "udp"
   token = "your_token" 
   connection_pool = 8
   aggressive_pool = false
   retry_interval = 3
   sniffer = false
   web_port = 2060 
   sniffer_log = "/root/backhaul.json"
   log_level = "info"

   ```
   
#### WebSocket Configuration
* **Server**:

   ```toml
   [[server]]
   bind_addr = "0.0.0.0:8080"
   transport = "ws"
   token = "your_token" 
   channel_size = 2048
   keepalive_period = 75 
   heartbeat = 40
   nodelay = true 
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   raw_ports = [
     "8080"
   ]
   ```

* **Client**:

   ```toml
   [[client]]
   remote_addr = "0.0.0.0:8080"
   edge_ip = "" 
   transport = "ws"
   token = "your_token" 
   connection_pool = 8
   aggressive_pool = false
   keepalive_period = 75 
   dial_timeout = 10
   retry_interval = 3
   nodelay = true 
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   ```

* **Details**:

   * Refer to TCP configuration for more information.

#### Secure WebSocket Configuration
* **Server**:

   ```toml
   [[server]]
   bind_addr = "0.0.0.0:8443"
   transport = "wss"
   token = "your_token" 
   channel_size = 2048
   keepalive_period = 75 
   nodelay = true 
   tls_cert = "/root/server.crt"      
   tls_key = "/root/server.key"
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   raw_ports = [
     "8080"
   ]
   ```

* **Client**:

   ```toml
   [[client]]
   remote_addr = "0.0.0.0:8443"
   edge_ip = "" 
   transport = "wss"
   token = "your_token" 
   connection_pool = 8
   aggressive_pool = false
   keepalive_period = 75
   dial_timeout = 10
   retry_interval = 3  
   nodelay = true 
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   ```

* **Details**:

   * Refer to the next section for instructions on generating `tls_cert` and `tls_key`.


#### WS Multiplexing Configuration
* **Server**:

   ```toml
   [[server]]
   bind_addr = "0.0.0.0:3080"
   transport = "wsmux"
   token = "your_token" 
   keepalive_period = 75
   nodelay = true 
   heartbeat = 40 
   channel_size = 2048
   mux_con = 8
   mux_version = 1
   mux_framesize = 32768 
   mux_recievebuffer = 4194304
   mux_streambuffer = 65536 
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   raw_ports = [
     "8080"
   ]
   ```
* **Client**:

   ```toml
   [[client]]
   remote_addr = "0.0.0.0:3080"
   edge_ip = "" 
   transport = "wsmux"
   token = "your_token" 
   connection_pool = 8
   aggressive_pool = false
   keepalive_period = 75
   dial_timeout = 10
   nodelay = true
   retry_interval = 3
   mux_version = 1
   mux_framesize = 32768 
   mux_recievebuffer = 4194304
   mux_streambuffer = 65536 
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   ```

#### WSS Multiplexing Configuration
* **Server**:

   ```toml
   [[server]]
   bind_addr = "0.0.0.0:443"
   transport = "wssmux"
   token = "your_token" 
   keepalive_period = 75
   nodelay = true 
   heartbeat = 40 
   channel_size = 2048
   mux_con = 8
   mux_version = 1
   mux_framesize = 32768 
   mux_recievebuffer = 4194304
   mux_streambuffer = 65536 
   tls_cert = "/root/server.crt"      
   tls_key = "/root/server.key"
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   raw_ports = [
     "8080"
   ]
   ```
* **Client**:

   ```toml
   [[client]]
   remote_addr = "0.0.0.0:443"
   edge_ip = "" 
   transport = "wssmux"
   token = "your_token" 
   keepalive_period = 75
   dial_timeout = 10
   nodelay = true
   retry_interval = 3
   connection_pool = 8
   aggressive_pool = false
   mux_version = 1
   mux_framesize = 32768 
   mux_recievebuffer = 4194304
   mux_streambuffer = 65536  
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   ```



## Raw Ports vs. SNI-based Routing

BackhaulPlus exposes two independent ways to accept user-facing inbound traffic
on a server. They can be used separately or together.

> **Each server must define at least one user-facing inbound:**
> - non-empty `raw_ports` for direct raw forwarding, or
> - `sni_router = true` with at least one valid `sni_routes` entry.
>
> `raw_ports` is optional, but it can only be omitted or left empty when
> `sni_router = true` and a valid `sni_routes` is defined. SNI-only servers do
> not need `raw_ports` — in that case, omit `raw_ports` entirely. A server with
> an empty `raw_ports` and no SNI router is rejected at startup with:
> `no inbound configured: set raw_ports or enable sni_router`.

### `raw_ports`

> The legacy `ports` field has been **removed**. Use `raw_ports` instead. If a
> configuration still contains `ports`, BackhaulPlus fails fast with:
> `field "ports" has been removed; use "raw_ports" instead`.

`raw_ports` is a list of ports that the BackhaulPlus server actually listens on
and forwards (raw TCP/UDP) into the tunnel. It accepts the same formats the old
`ports` field did:

```toml
raw_ports = [
  "20000-20100"               # Listen on every port in 20000..20100, forward each to the same remote port
]
```

### `sni_router`

`sni_router` enables an internal SNI-based TCP router. It is a TCP listener that
reads the TLS **ClientHello** of each incoming connection **without terminating
TLS** (no certificate is required, no traffic is decrypted) and routes the
connection into the tunnel based on the SNI value. The inspected ClientHello
bytes are preserved and replayed to the destination, so TLS/REALITY/XHTTP
handshakes keep working end-to-end.

This removes the need for an extra HAProxy/Nginx `stream` hop in front of
BackhaulPlus: there is **no internal `127.0.0.1:10001` dial**, no extra loopback
hop, and no second socket pair — the same accepted connection is fed directly
into the tunnel transport, exactly like raw port forwarding, but without a real
raw listener for that route.

Configuration fields:

| Field | Meaning |
| --- | --- |
| `sni_router` | Enable the SNI router (`true`/`false`, default `false`). |
| `sni_listen_addr` | Address the SNI router listens on (e.g. `0.0.0.0:443`). Required when `sni_router = true`. |
| `sni_inspect_timeout` | Seconds allowed to read the ClientHello. Defaults to `1` if `<= 0`. |
| `sni_default_action` | Action for unknown SNIs. Only `reject` is currently supported (default). |
| `sni_routes` | Array of `{ sni = "...", target = "..." }` rules mapping an exact SNI host to a virtual tunnel target. |

Important notes:

* The targets inside `sni_routes` (e.g. `"10001"`) are **virtual tunnel
  targets**. They are sent to the client unchanged and do **not** need a matching
  entry in `raw_ports`. No listener is opened on the server for those ports.
* The client/middle side does not know whether a connection arrived via
  `raw_ports` or `sni_routes`; it receives the same target string and connects
  to its destination with its existing logic.
* SNI keys are normalized (trimmed, lowercased, trailing dot removed) and matched
  case-insensitively. Only exact matches are supported (no wildcards/regex yet).
* If an SNI does not match any route and `sni_default_action = "reject"`, the
  connection is closed.
* Per-route traffic in the usage monitor is reported using the target port
  (e.g. `myket.ir → 10001`, `cafebazaar.ir → 10002`), so each SNI route is
  accounted separately even though they all arrive on the SNI listener port.

#### Example 1: raw forwarding only

```toml
[[server]]
name = "raw-example"
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
token = "example-token"

raw_ports = [
  "10000-10100"
]
```

The server listens on ports 10000–10100 and forwards each into the tunnel. No
SNI router is started.

#### Example 2: SNI router only

```toml
[[server]]
name = "sni-example"
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
token = "example-token"

sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_inspect_timeout = 1
sni_default_action = "reject"

sni_routes = [
  { sni = "myket.ir", target = "10001" },
  { sni = "cafebazaar.ir", target = "10002" },
  { sni = "telewebion.ir", target = "10003" }
]
```

Here the server listens only on `0.0.0.0:443` and `raw_ports` is omitted
entirely. A TLS connection with SNI `myket.ir` enters the tunnel with target
`10001`, `cafebazaar.ir` with `10002`, and `telewebion.ir` with `10003`. No
listener is opened on 10001/10002/10003.

#### Example 3: raw forwarding + SNI router

```toml
[[server]]
name = "mixed-example"
bind_addr = "0.0.0.0:30000"
transport = "tcpmux"
token = "example-token"

raw_ports = [
  "20000-20100"
]

sni_router = true
sni_listen_addr = "0.0.0.0:443"
sni_inspect_timeout = 1
sni_default_action = "reject"

sni_routes = [
  { sni = "myket.ir", target = "10001" },
  { sni = "cafebazaar.ir", target = "10002" },
  { sni = "telewebion.ir", target = "10003" }
]
```

`raw_ports` handles real raw forwarding (20000–20100) while `sni_routes` routes
TLS connections on `:443` by SNI — completely independent of `raw_ports`.

### Transport support

The SNI router is transport-agnostic and works with every stream-based
transport: `tcp`, `tcpmux`, `ws`, `wss`, `wsmux`, `wssmux`, and `quic` (its
local TCP inbound path). It feeds routed connections into the same internal
pipeline as raw port forwarding.

The `udp` transport is the only exception: it only carries real UDP datagrams
and cannot transport a TCP/TLS inbound stream. Enabling `sni_router` with
`transport = "udp"` is rejected at startup with a clear error.

### Validation rules

A server entry is rejected at startup if:

* it has neither `raw_ports` nor `sni_router` (no user-facing inbound);
* `sni_router = true` but `sni_listen_addr` is empty;
* `sni_router = true` but `sni_routes` is empty;
* `sni_default_action` is set to anything other than `reject`;
* `transport = "udp"` is combined with `sni_router = true`;
* it still uses the removed `ports` field.

## Generating a Self-Signed TLS Certificate with OpenSSL

To generate a TLS certificate and key, you can use tools like OpenSSL. Here’s a step-by-step guide on how to create a self-signed certificate and key using OpenSSL:

### Step 1: Install OpenSSL

If you don't already have OpenSSL installed, you can install it using your system's package manager.

- **On Ubuntu/Debian**:
  ```bash
  sudo apt-get install openssl
  ```
### Step 2: Generate a Private Key
To generate a 2048-bit RSA private key, run the following command:
  ```bash
openssl genpkey -algorithm RSA -out server.key -pkeyopt rsa_keygen_bits:2048
  ```
This will create a file named `server.key`, which is your private key.
### Step 3: Generate a Certificate Signing Request (CSR)

Create a Certificate Signing Request (CSR) using the private key. This CSR is used to generate the SSL certificate:
  ```bash
openssl req -new -key server.key -out server.csr
  ```

You will be prompted to enter information for the CSR. For the common name (CN), use the domain name or IP address where your server will be hosted. Example:
```
Country Name (2 letter code) [AU]:US
State or Province Name (full name) [Some-State]:California
Locality Name (eg, city) []:San Francisco
Organization Name (eg, company) [Internet Widgits Pty Ltd]:Your Company Name
Organizational Unit Name (eg, section) []:
Common Name (e.g. server FQDN or YOUR name) []:example.com
Email Address []:
```

### Step 4: Generate a Self-Signed Certificate

Use the CSR and private key to generate a self-signed certificate. Specify the validity period (in days):
  ```bash
openssl x509 -req -in server.csr -signkey server.key -out server.crt -days 365
  ```
This will generate a certificate named `server.crt`, valid for 365 days.
### Recap of the Files Generated:

* `server.key`: Your private key.
* `server.csr`: The certificate signing request (used to generate the certificate).
* `server.crt`: Your self-signed TLS certificate.

## Running BackhaulPlus as a service

To create a service file for your BackhaulPlus project that ensures the service restarts automatically, you can use the following template for a systemd service file. Assuming your project runs a reverse tunnel and the main executable file is located in a certain path, here's a basic example:

1. Create the service file `/etc/systemd/system/BackhaulPlus.service`:

```ini
[Unit]
Description=BackhaulPlus Reverse Tunnel Service
After=network.target

[Service]
Type=simple
ExecStart=/root/BackhaulPlus/BackhaulPlus -c /root/BackhaulPlus/config.toml
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```
2. After creating the service file, enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable BackhaulPlus.service
sudo systemctl start BackhaulPlus.service
```
3. To verify if the service is running:
```bash
sudo systemctl status BackhaulPlus.service
```
4. View the most recent log entries for the BackhaulPlus.service unit:
```bash
journalctl -u BackhaulPlus.service -e -f
```

## FAQ

**Q: How do I decide which transport protocol to use?**

* `tcp`: Use if you need straightforward TCP connections.
* `tcpmux`: Use if you need to handle multiple sessions over a single connection.
* `ws`: Use if you need to traverse HTTP-based firewalls or proxies.
* `wss`: Use this for secure WebSocket connections that need to traverse HTTP-based firewalls or proxies. It encrypts data for added security, similar to WS but with encryption.


## Benchmark

For in-depth information, please visit the dedicated [Benchmark page](./benchmark/).


## License

This project is licensed under the AGPL-3.0 license. See the LICENSE file for details.

## Donation

Donate TRX (TRC-20) to support our project:
``` wallet
TKkzfx6GVnARFLpgALXCEyxVpQHTnwtAJt
```
Thanks for your support! 

## Stargazers over time
[![Stargazers over time](https://starchart.cc/codeTide/BackhaulPlus.svg?variant=light)](https://starchart.cc/codeTide/BackhaulPlus)

