FROM node:22.22.0-bookworm-slim@sha256:dd9d21971ec4395903fa6143c2b9267d048ae01ca6d3ea96f16cb30df6187d94 AS node
FROM oven/bun:1.3.14 AS bun
FROM rust:1.94-bookworm AS toolchain

# A reproducible build/QA substrate, never a production server or container.
COPY --from=node /usr/local/ /usr/local/
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
RUN apt-get update \
    && apt-get install -y --no-install-recommends python3 ca-certificates pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund \
    && rustup target add --toolchain 1.94.0 wasm32-unknown-unknown \
    && cargo +1.94.0 install --locked --version =0.8.5 --root /src/target/cloudflare-tools worker-build \
    && cargo +1.94.0 install --locked --version =0.2.125 --root /src/target/cloudflare-tools wasm-bindgen-cli

FROM toolchain AS bundle
COPY . .
ARG SCRY_REVISION
RUN python3 scripts/scry-cloudflare build --revision "$SCRY_REVISION" --out /artifact

# Export with BuildKit's local exporter. There is deliberately no native runtime.
FROM scratch AS artifact
COPY --from=bundle /artifact/ /
