# Build stage
FROM golang:1.25.4-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o duet .

# Run stage
FROM alpine:latest

RUN apk add --no-cache bash coreutils \
    && echo "alias ls='ls --color=auto'" >> /root/.bashrc \
    && echo "alias grep='grep --color=auto'" >> /root/.bashrc \
    && echo "export GREP_OPTIONS='--color=auto'" >> /root/.bashrc \
    && echo "git config --global color.ui auto" >> /root/.bashrc \
    && echo "export TERM=xterm-256color" >> /root/.bashrc

RUN apk add --no-cache openssh

RUN adduser -D duet
WORKDIR /app

# Generate SSH host key before switching to non-root user
RUN mkdir -p /app/.ssh && \
    ssh-keygen -t ed25519 -f /app/.ssh/id_ed25519 -N "" && \
    chown -R duet:duet /app/.ssh

# Create workspace template for chroot jailing
RUN mkdir -p /app/workspace-template/{bin,lib,usr/bin,home/duet,tmp,dev} && \
    cp /bin/sh /bin/bash /bin/ls /bin/cat /bin/mkdir /bin/rm /bin/cp /bin/mv /bin/echo /bin/vi /app/workspace-template/bin/ 2>/dev/null || true && \
    cp /lib/ld-musl-*.so.1 /app/workspace-template/lib/ 2>/dev/null || true && \
    chmod 1777 /app/workspace-template/tmp && \
    mkdir -p /app/workspaces && \
    chown -R duet:duet /app

COPY --from=builder /app/duet /app/duet

# Grant chroot capability so duet can chroot without being root
RUN apk add --no-cache libcap && \
    setcap cap_sys_chroot+ep /app/duet

USER duet

# Expose the internal port your app listens on
EXPOSE 2222

CMD ["/app/duet", "-addr", ":2222", "-hostkey", "/app/.ssh/id_ed25519", "-worker", "https://duet-cf-worker.incident-agent.workers.dev"]
