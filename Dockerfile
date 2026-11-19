FROM debian:13-slim

# Network troubleshooting toolbox image for Kubernetes debugging pods.
# Install a broad set of diagnostic utilities in a single layer.
RUN apt-get update && apt-get install -y --no-install-recommends \
        # --- Connectivity, routing & interfaces ---
        iproute2 \
        iputils-ping \
        iputils-tracepath \
        iputils-arping \
        traceroute \
        mtr-tiny \
        net-tools \
        ethtool \
        # --- DNS ---
        dnsutils \
        ldnsutils \
        # --- HTTP / TLS / transfer ---
        curl \
        wget \
        openssl \
        ca-certificates \
        # --- Packet capture & analysis ---
        tcpdump \
        tshark \
        ngrep \
        # --- Sockets, ports, scanning & throughput ---
        socat \
        netcat-openbsd \
        nmap \
        iperf3 \
        nftables \
        # --- General diagnostics & convenience ---
        jq \
        yq \
        procps \
        lsof \
        strace \
        htop \
        iputils-clockdiff \
        dnsmasq-base \
        zsh \
        vim  less gnupg
#        less \
#    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://pgp.mongodb.com/server-8.0.asc | gpg --dearmor -o /usr/share/keyrings/mongodb-server-8.0.gpg
RUN echo "deb [signed-by=/usr/share/keyrings/mongodb-server-8.0.gpg] https://repo.mongodb.org/apt/debian trixie/mongodb-org/8.0 main" | tee /etc/apt/sources.list.d/mongodb-org-8.0.list
RUN apt update
RUN apt install -y mongodb-mongosh
RUN mongosh --version

# Make zsh the default interactive shell.
ENV SHELL=/usr/bin/zsh
RUN chsh -s /usr/bin/zsh root

# Login banner: printed for interactive shells (e.g. `kubectl exec -it ... -- zsh`).
COPY motd.sh /etc/profile.d/zz-tsp-motd.sh
RUN chmod +x /etc/profile.d/zz-tsp-motd.sh \
    # Interactive zsh reads /etc/zsh/zshrc; interactive non-login bash reads
    # /etc/bash.bashrc. Source the banner from both so either shell shows it.
    && printf '\n# TSP troubleshooting-pod banner\nif [ -n "$PS1" ]; then . /etc/profile.d/zz-tsp-motd.sh; fi\n' \
       | tee -a /etc/zsh/zshrc >> /etc/bash.bashrc

# Keep the pod alive so you can `kubectl exec` into it.
CMD ["sleep", "infinity"]
