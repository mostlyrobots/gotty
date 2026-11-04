# GoTTY - Development Sandbox Container
# Provides a development environment on Ubuntu 24.04 LTS
# Authors: Google Gemini 4 Argon & the original GoTTY project

FROM docker.io/library/ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

# Install Google Cloud Shell-style essential packages and dev tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash-completion \
    build-essential \
    bzip2 \
    ca-certificates \
    curl \
    dnsutils \
    file \
    git \
    htop \
    iproute2 \
    iputils-ping \
    jq \
    less \
    nano \
    net-tools \
    procps \
    python3 \
    python3-pip \
    python3-venv \
    sudo \
    tar \
    tree \
    unzip \
    vim-tiny \
    wget \
    xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Remove default ubuntu user so arbitrary UID/GIDs can be assigned dynamically
RUN touch /var/mail/ubuntu && userdel -r ubuntu 2>/dev/null || true

# Allow world-writable /etc/passwd & /etc/group for arbitrary container UIDs
RUN chmod 666 /etc/passwd /etc/group

# Allow passwordless sudo for all users inside the container sandbox
RUN echo "ALL ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/nopasswd && \
    echo "Defaults !authenticate" >> /etc/sudoers.d/nopasswd && \
    chmod 0440 /etc/sudoers.d/nopasswd

# Configure PAM for sudo so dynamic accounts validate cleanly
RUN printf "#%%PAM-1.0\nauth sufficient pam_permit.so\naccount sufficient pam_permit.so\nsession sufficient pam_permit.so\n" > /etc/pam.d/sudo

# Set prompt to match Cloud Shell style and ensure current user is registered in /etc/passwd
RUN echo 'if ! whoami >/dev/null 2>&1; then echo "${USER:-user}:x:$(id -u):$(id -g):${USER:-user}:${HOME:-/home/user}:/bin/bash" >> /etc/passwd 2>/dev/null || true; fi' >> /etc/bash.bashrc && \
    echo "export PS1='\[\033[01;32m\]\u@sandbox\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ '" >> /etc/bash.bashrc

# Copy godl file download helper into container PATH
COPY godl /usr/local/bin/godl
RUN chmod 0755 /usr/local/bin/godl

WORKDIR /root
CMD ["/bin/bash"]
