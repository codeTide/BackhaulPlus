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
5. [Ports, SNI Gateways, and HTTP Gateways](#ports-sni-gateways-and-http-gateways)
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

* **Runtime Maintenance (top-level, optional)**

   These options are process-wide. They apply to the whole BackhaulPlus process, whether it is running as a client or a server, and are disabled by default unless explicitly configured.

   ```toml
   [runtime]
   memory_release_interval = "0" # Periodically ask the runtime to release idle memory. Disabled by "0" or when omitted. Examples: "5m", "10m", "1h".
   auto_restart_interval = "0"   # Automatically re-exec the BackhaulPlus process after this interval. Disabled by "0" or when omitted. Examples: "1h", "6h", "24h".
   ```

   Notes:

   * `memory_release_interval` may reduce RSS when the Go runtime has idle heap memory, but it cannot free memory that is still actively used by live connections, goroutines, buffers, or smux sessions.
   * Very short `memory_release_interval` values may increase CPU usage or latency because memory release forces GC work. Values below `1s` are rejected.
   * `auto_restart_interval` re-execs the current process and drops active connections; graceful drain is not implemented for this maintenance action. Values below `1m` are rejected to avoid rapid restart loops.
   * Both options are disabled by default and are only enabled when explicitly configured.

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
    tcp_copy_buffer = "16kb"      # Userspace buffer used by TCPConnectionHandler copy paths (tcp, tcpmux, wsmux/wssmux). Lower values reduce memory under high connection counts; higher values may improve throughput. Default: "16kb".
    sniffer = false               # Enable or disable network sniffing for monitoring data. (optional, default false)
    web_port = 2060               # Port number for the web interface or monitoring interface. (optional, set to 0 to disable).
    sniffer_log = "/root/log.json" # Filename used to store network traffic and usage data logs. (optional, default backhaul.json)
    tls_cert = "/root/server.crt" # Path to the TLS certificate file for wss/wssmux. (mandatory for wss/wssmux).
    tls_key = "/root/server.key"  # Path to the TLS private key file for wss/wssmux. (mandatory for wss/wssmux).
    log_level = "info"            # Log level ("panic", "fatal", "error", "warn", "info", "debug", "trace", optional, default: "info").

    # NOTE: the field is named `ports`. The old `raw_ports` name has been
    # removed; a config still using `raw_ports` fails fast at startup.
    ports = [
      "443-600",                  # Listen on all ports in the range 443 to 600
      "443-600=5201",             # Listen on all ports in the range 443 to 600 and forward traffic to 5201
      "443-600=1.1.1.1:5201",     # Listen on all ports in the range 443 to 600 and forward traffic to 1.1.1.1:5201
      "443",                      # Listen on local port 443 and forward to remote port 443 (default forwarding).
      "4000=5000",                # Listen on local port 4000 (bind to all local IPs) and forward to remote port 5000.
      "127.0.0.2:443=5201",       # Bind to specific local IP (127.0.0.2), listen on port 443, and forward to remote port 5201.
      "443=1.1.1.1:5201",         # Listen on local port 443 and forward to a specific remote IP (1.1.1.1) on port 5201.
      "127.0.0.2:443=1.1.1.1:5201" # Bind to specific local IP (127.0.0.2), listen on port 443, and forward to remote IP (1.1.1.1) on port 5201.
    ]
    ```

    SNI-based routing is now configured in a standalone `[[sni_gateway]]`
    section instead of per-server fields. See
    [Ports, SNI Gateways, and HTTP Gateways](#ports-sni-gateways-and-http-gateways) below.

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
   retry_interval = 3            # Retry interval between control-channel reconnect attempts. Accepts a bare number of seconds (e.g. 3), a fixed duration ("5s", "500ms"), or an adaptive backoff range ("5s-60s"). (optional, default: 3s).
   dial_rate_limit = "2/s"       # Cap on new remote Dial/connect attempts per second for this client. Empty/"0"/"0/s" disable it. Only throttles remote dials, never local Xray/localhost dials. (optional, default: disabled).
   dial_timeout = 10             # Sets the max wait time for establishing a network connection. (optional, default: 10s)
   mux_version = 1               # SMUX protocol version (1 or 2). Version 2 may have extra features. (optional)
   mux_framesize = 32768         # 32 KB. The maximum size of a frame that can be sent over a connection. (optional)
   mux_recievebuffer = 4194304   # 4 MB. The maximum buffer size for incoming data per connection. (optional)
   mux_streambuffer = 65536      # 256 KB. The maximum buffer size per individual stream within a connection. (optional)
   tunnel_tcp_buffer = "2mb"     # tcpmux only. TCP socket buffer for tunnel connections. "auto" lets the OS/kernel autotune; "2mb" keeps the old behavior. Examples: "auto", "512kb", "1mb", "2mb", "524288". (optional, default: "2mb")
   tcp_copy_buffer = "16kb"      # Userspace buffer used by TCPConnectionHandler copy paths (tcp, tcpmux, wsmux/wssmux). Lower values reduce memory under high connection counts; higher values may improve throughput. Default: "16kb".
   sniffer = false               # Enable or disable network sniffing for monitoring data. (optional, default false)
   web_port = 2060               # Port number for the web interface or monitoring interface. (optional, set to 0 to disable).
   sniffer_log ="/root/log.json" # Filename used to store network traffic and usage data logs. (optional, default backhaul.json)
   log_level = "info"            # Log level ("panic", "fatal", "error", "warn", "info", "debug", "trace", optional, default: "info").
   ```

   For the `tcpmux` transport, `tunnel_tcp_buffer` controls the TCP socket
   receive/send buffer applied to each tunnel connection to the server:

   * `"auto"` lets the OS/kernel autotune the TCP buffers (no explicit override).
   * `"512kb"`, `"1mb"`, `"2mb"` set both the receive and send socket buffers to a
     fixed size (`kb` = 1024 bytes, `mb` = 1024 × 1024 bytes). A raw byte count
     such as `"524288"` is also accepted.
   * `"2mb"` is the default and preserves the historical behavior. If
     `tunnel_tcp_buffer` is omitted, the old 2MB behavior is kept, so existing
     configs are unaffected.

   On systems with many tunnel/connections and memory pressure, try
   `tunnel_tcp_buffer = "auto"` or `tunnel_tcp_buffer = "512kb"`.

   `tcp_copy_buffer` is different from `tunnel_tcp_buffer` and applies to both
   `[[server]]` and `[[client]]` blocks:

   * `tunnel_tcp_buffer` is the **kernel TCP socket buffer** of the `tcpmux`
     tunnel connection.
   * `tcp_copy_buffer` is the **userspace buffer** inside `transferData` that
     `TCPConnectionHandler` uses to copy data between the tunnel/stream and the
     local TCP connection. It applies to all transport paths that use
     `TCPConnectionHandler`, including `tcp`, `tcpmux`, and `wsmux`/`wssmux`. It
     is not a kernel socket buffer. (Plain `ws`/`wss` use a separate WebSocket
     handler and are not affected by this option.)

   Accepted values are `"1kb"`–`"1mb"` (binary units: `kb` = 1024 bytes, `mb` =
   1024 × 1024 bytes) or a raw byte count such as `"4096"`. The default is
   `"16kb"`, which preserves the historical behavior, so existing configs are
   unaffected. Each active connection has roughly two copy directions, so under
   high connection counts a smaller buffer can save a lot of memory:

   ```toml
   tcp_copy_buffer = "4kb"
   ```

   or:

   ```toml
   tcp_copy_buffer = "2kb"
   ```

   ### Adaptive retry and remote dial rate limiting (client)

   These two client options help when the destination server (e.g. an Iran
   endpoint reached from an OVH VPS) becomes unreachable, dropped or
   blackholed for a while, and you want to avoid a reconnect/SYN storm:

   * `retry_interval` controls how long the client waits between failed
     attempts to (re)establish its remote control channel. It is
     backward-compatible and accepts three forms:
     * a bare number — `retry_interval = 5` means a **fixed 5 seconds**
       (legacy behavior, still valid);
     * a fixed duration string — `retry_interval = "5s"`, `"500ms"`,
       `"1m"`, `"2m30s"` (standard Go `time.ParseDuration` units);
     * an adaptive backoff range — `retry_interval = "5s-60s"` starts at
       `5s` and doubles after each consecutive failure (`5s, 10s, 20s, 40s,
       60s, 60s, …`) up to the maximum, then resets to the minimum once a
       connection succeeds. For a range, the start must be greater than `0`
       and strictly smaller than the maximum.

   * `dial_rate_limit` caps how many **new remote Dial/connect attempts**
     this client makes towards `remote_addr` per second, e.g.
     `dial_rate_limit = "2/s"`. An empty value, `"0"` or `"0/s"` (or omitting
     the option) disables it and keeps the old behavior. The limiter is
     **shared across all dial goroutines of the same client** (control
     channel, connection pool and load dials), so the per-second cap is
     enforced globally for that client.

     `dial_rate_limit` limits BackhaulPlus remote Dial attempts. It does not
     directly limit kernel TCP SYN packets, but it reduces SYN bursts by
     limiting new connect attempts. It is **only** applied to remote dials to
     `remote_addr`; it is never applied to local Xray/`localhost` dials. It is
     applied to the remote dials of all client transports (`tcp`, `tcpmux`,
     `ws`/`wss`, `wsmux`/`wssmux`, `quic`, `udp`).

   Example tuned for a provider sensitive to outbound SYN bursts:

   ```toml
   [[client]]
   name = "IR1"
   remote_addr = "87.107.83.36:20017"
   transport = "tcpmux"
   token = "CHANGE_ME"

   connection_pool = 8
   aggressive_pool = false
   dial_timeout = 10

   # Backward-compatible fixed retry:
   # retry_interval = 5

   # Adaptive retry: start at 5s and back off up to 60s after repeated failures.
   retry_interval = "5s-60s"

   # Limit outbound remote Dial attempts from this client.
   dial_rate_limit = "2/s"
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
   name = "SRV1"
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
   ports = [
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
   name = "SRV1"
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
   ports = [
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
   tunnel_tcp_buffer = "2mb"
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
   name = "SRV1"
   bind_addr = "0.0.0.0:3080"
   transport = "udp"
   token = "your_token"
   heartbeat = 20 
   channel_size = 2048
   sniffer = false 
   web_port = 2060
   sniffer_log = "/root/backhaul.json"
   log_level = "info"
   ports = [
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
   name = "SRV1"
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
   ports = [
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
   name = "SRV1"
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
   ports = [
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
   name = "SRV1"
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
   ports = [
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
   name = "SRV1"
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
   ports = [
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



## Ports, SNI Gateways, and HTTP Gateways

BackhaulPlus accepts user-facing inbound traffic in three independent ways:

* **`ports`** on a `[[server]]` — raw TCP/UDP port forwarding into that server's
  tunnel.
* **`[[sni_gateway]]`** — a standalone, shared public listener (e.g.
  `0.0.0.0:443`) that routes **encrypted TLS** connections to the correct server
  by their ClientHello **SNI**, **without terminating TLS**.
* **`[[http_gateway]]`** — a standalone, shared public listener that routes
  **cleartext HTTP/1.x** connections to the correct server by their **Host**
  header. Use this for HTTP/XHTTP that is *not* wrapped in TLS.

`sni_gateway` vs `http_gateway` at a glance:

| | `sni_gateway` | `http_gateway` |
| --- | --- | --- |
| Pre-reads | TLS ClientHello | HTTP/1.x request header |
| Routes by | SNI | `Host` header |
| Use for | TLS / REALITY | cleartext HTTP / XHTTP-over-HTTP |
| Terminates TLS? | No | No |
| Works for TLS/REALITY? | Yes | No — the Host is encrypted, use `sni_gateway` |

Both gateways are transport-agnostic, preserve the first bytes they read (via
`PrefixedConn`) and never decrypt traffic; they only differ in what plaintext
header they parse to pick a route.

> **Breaking changes**
> - `raw_ports` has been removed; use `ports`. A config still containing
>   `raw_ports` fails fast with:
>   `field "raw_ports" has been removed; use "ports" instead`.
> - Per-server SNI routing (`sni_router`, `sni_listen_addr`,
>   `sni_inspect_timeout`, `sni_default_action`, `sni_routes`) has been removed;
>   use `[[sni_gateway]]`. A config still containing any of those fields fails
>   fast with: `per-server sni_router has been removed; use [[sni_gateway]] instead`.

### `ports`

`ports` is a list of ports that the server actually listens on and forwards
(raw TCP/UDP) into its tunnel. Supported formats:

```toml
ports = [
  "443-600",                   # Listen on every port in 443..600, forward each to the same remote port
  "443-600=5201",              # Range, forward all to remote port 5201
  "443-600=1.1.1.1:5201",      # Range, forward all to 1.1.1.1:5201
  "443",                       # Single port, forward to remote port 443
  "4000=5000",                 # Listen on 4000, forward to remote 5000
  "127.0.0.2:443=5201",        # Bind a specific local IP, forward to remote 5201
  "443=1.1.1.1:5201",          # Listen on 443, forward to 1.1.1.1:5201
  "127.0.0.2:443=1.1.1.1:5201" # Bind a specific local IP, forward to 1.1.1.1:5201
]
```

### `[[sni_gateway]]`

A **SNI gateway** is a standalone section that is intentionally decoupled from
`[[server]]` blocks. It opens **one** public TCP listener (e.g. `0.0.0.0:443`),
reads the TLS **ClientHello** of each incoming connection **without terminating
TLS** (no certificate is required, no traffic is decrypted), extracts the SNI,
and dispatches the connection — with the inspected ClientHello bytes preserved
and replayed — into the tunnel of the routed `[[server]]`.

Why a separate section? Because several external servers (e.g. `TR1` and `US1`)
can sit behind a **single** public `IP:443` entrypoint. Each keeps its own
tunnel and its own client; the gateway only decides, per SNI, which server's
runtime should receive the connection. Without this, two server blocks both
trying to listen on `0.0.0.0:443` would fail with `address already in use`.

A gateway never terminates TLS: only the ClientHello is pre-read, the first
bytes are preserved with an internal `PrefixedConn`, and the original stream is
handed off intact so TLS/REALITY/XHTTP handshakes keep working end-to-end.

Configuration fields:

| Field | Meaning |
| --- | --- |
| `name` | Label for this gateway, used in logs. |
| `listen_addr` | The single public address the gateway listens on (e.g. `0.0.0.0:443`). Required, must be unique, and must not collide with any server `ports` listener. |
| `inspect_timeout` | Seconds allowed to read the ClientHello. Defaults to `1` if `<= 0`. |
| `default_action` | Action for unknown SNIs. Only `reject` is currently supported (default). |
| `routes` | Array of `{ sni = "...", server = "...", target = "..." }` rules. |

Route fields:

* `sni` — the exact SNI host to match. Normalized (trimmed, lowercased, trailing
  dot removed) and matched case-insensitively. No wildcards/regex yet.
* `server` — the `name` of the `[[server]]` whose tunnel receives the connection.
* `target` — the target string sent to the external client (e.g. `"443"`). It is
  the destination the client connects to and does **not** need a matching
  `ports` entry on the server. When `target` is numeric, per-route traffic in
  the usage monitor is reported under that port, so each SNI route is accounted
  separately even though they all arrive on the gateway listener.

### Multi-server example

```toml
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
token = "example-token"

ports = [
  "64335=64335"
]

[[server]]
name = "US1"
bind_addr = "0.0.0.0:20002"
transport = "tcpmux"
token = "example-token"

ports = [
  "64336=64335"
]

[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
inspect_timeout = 1
default_action = "reject"

routes = [
  { sni = "tr.example.com", server = "TR1", target = "443" },
  { sni = "us.example.com", server = "US1", target = "443" }
]
```

Behaviour on the Iran-side server (`IR1`):

```text
IR1:443 + SNI tr.example.com → TR1 → target 443
IR1:443 + SNI us.example.com → US1 → target 443
IR1:64335 → TR1 → target 64335
IR1:64336 → US1 → target 64335
```

Only the `[[sni_gateway]]` binds `0.0.0.0:443`; `TR1` and `US1` each keep their
own tunnel and their own dedicated `ports`, so there is no port conflict.

### SNI-only server (no `ports`)

A server may have **no** `ports` of its own as long as at least one gateway
route references it:

```toml
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
token = "example-token"

[[sni_gateway]]
name = "PUBLIC-443"
listen_addr = "0.0.0.0:443"
routes = [
  { sni = "a.com", server = "TR1", target = "443" }
]
```

### `[[http_gateway]]`

An **HTTP gateway** is the cleartext sibling of `sni_gateway`. It opens **one**
public TCP listener, reads each connection's **cleartext HTTP/1.x request
header** (only up to `\r\n\r\n`, never the body), extracts the **Host** header,
and dispatches the connection — with the inspected bytes preserved and replayed
— into the tunnel of the routed `[[server]]`. TLS is never terminated and no
certificate is required.

This is for XHTTP/HTTP setups that arrive as plain HTTP, e.g.:

```http
POST /xhttp HTTP/1.1
Host: tr.example.com
...
```

```http
GET / HTTP/1.1
host: us.example.com
...
```

Configuration fields:

| Field | Meaning |
| --- | --- |
| `name` | Label for this gateway, used in logs. |
| `listen_addr` | The single public address the gateway listens on. Required, unique, and must not collide with any other listener. |
| `inspect_timeout` | Seconds allowed to read the request header. Defaults to `1` if `<= 0`. |
| `max_header_bytes` | Max bytes read while looking for the header terminator. Defaults to `32768`; must be between `128` and `1048576`. |
| `default_action` | Action for unknown Hosts. Only `reject` is currently supported (default). |
| `routes` | Array of `{ host = "...", server = "...", target = "..." }` rules. |

Route fields:

* `host` — the exact Host to match. Normalized (trimmed, lowercased, `:port`
  stripped, trailing dot removed) and matched case-insensitively. So
  `Example.COM:443` and `example.com.` both match `example.com`. No
  wildcards/regex yet.
* `server` — the `name` of the `[[server]]` whose tunnel receives the connection.
* `target` — the target string sent to the external client (e.g. `"443"`). When
  numeric, per-route traffic is reported under that port.

> **Scope (v1):** `http_gateway` only supports cleartext **HTTP/1.1** (and
> HTTP/1.0) request preread and `Host`-based routing. It does **not** support
> TLS, REALITY, HTTPS Host routing, HTTP/2, h2c, `:authority`, or path/method
> routing. An HTTP/2 preface (`PRI * HTTP/2.0`) is rejected. For TLS/REALITY,
> use `[[sni_gateway]]` instead.

#### Multi-server HTTP example

```toml
[[server]]
name = "TR1"
bind_addr = "0.0.0.0:20001"
transport = "tcpmux"
token = "example-token"

[[server]]
name = "US1"
bind_addr = "0.0.0.0:20002"
transport = "tcpmux"
token = "example-token"

[[http_gateway]]
name = "PUBLIC-XHTTP-443"
listen_addr = "0.0.0.0:443"
inspect_timeout = 1
max_header_bytes = 32768
default_action = "reject"

routes = [
  { host = "tr.example.com", server = "TR1", target = "443" },
  { host = "us.example.com", server = "US1", target = "443" }
]
```

```text
IR1:443 + HTTP Host tr.example.com → TR1 → target 443
IR1:443 + HTTP Host us.example.com → US1 → target 443
```

`sni_gateway` and `http_gateway` are complementary and each needs its **own**
`listen_addr`: two listeners cannot share one `IP:port`, so defining both on the
same port is a validation error. This PR does not multiplex protocols on one
port.

### Transport support

Both SNI and HTTP gateways are transport-agnostic and dispatch to any
stream-based transport: `tcp`, `tcpmux`, `ws`, `wss`, `wsmux`, `wssmux`, and
`quic`. The `udp` transport is the only exception — it carries real UDP
datagrams and cannot serve a TCP stream — so a route pointing at a `udp` server
is rejected at startup.

### Validation rules

Configuration is validated before any listener starts. It is rejected if:

* a `[[server]]` has an empty `name`, or two servers share the same `name`;
* a server has neither `ports` nor any `[[sni_gateway]]`/`[[http_gateway]]`
  route referencing it
  (`no inbound configured for server "TR1": set ports or reference it from [[sni_gateway]] or [[http_gateway]].routes`);
* a `ports` entry has an invalid format;
* two servers bind the same port (`duplicate port listener ...`) or the same
  `bind_addr` (`duplicate server bind_addr ...`);
* a `[[sni_gateway]]` or `[[http_gateway]]` has an empty `listen_addr`;
* any two gateways share the same `listen_addr` (including one `sni_gateway` and
  one `http_gateway`), or a gateway `listen_addr` collides with a server
  `bind_addr` or `ports` listener — including wildcard/specific-IP overlaps such
  as `0.0.0.0:443` vs `127.0.0.1:443`;
* an `http_gateway` `max_header_bytes` is outside `128..1048576`;
* a route is missing its key (`sni`/`host`), `server`, or `target`;
* two routes in the same gateway resolve to the same normalized SNI/Host;
* a route references an unknown server
  (`sni_gateway "PUBLIC-443" route for "example.com" references unknown server "TR2"`);
* a route references a `udp` server;
* `default_action` is set to anything other than `reject`;
* the config still uses the removed `raw_ports` or per-server `sni_*` fields.

> The same SNI or Host may repeat across gateways that listen on **different**
> `listen_addr` values (separate public entrypoints), but each `listen_addr`
> must be unique, which also prevents two gateways from sharing one public port.

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

