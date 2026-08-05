FROM ghcr.io/konono/aw-opencode:4.7.0-debian12

USER agent

# mise で Go をグローバルインストール
RUN mise use -g go@1.25 && mise reshim

# mise shims を PATH に追加（Docker build 時は BASH_ENV が効かないため明示）
ENV GOBIN="/home/agent/.local/bin"
ENV PATH="/home/agent/.local/share/mise/shims:${GOBIN}:${PATH}"

# trimetry インストール
RUN go install github.com/konono/trimetry/cmd/trimetry@latest

# opencode langfuse プラグインをイメージにベイク
# @konono スコープは GitHub Packages にあるため npmrc 設定が必要
ARG GITHUB_TOKEN=""
RUN echo "@konono:registry=https://npm.pkg.github.com/" >> ~/.npmrc && \
    echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" >> ~/.npmrc && \
    mkdir -p /tmp/plugin-setup/.opencode && \
    echo '{}' > /tmp/plugin-setup/.opencode/opencode.json && \
    cd /tmp/plugin-setup && \
    opencode plugin @konono/opencode-plugin-langfuse -f && \
    rm -rf /tmp/plugin-setup ~/.npmrc

# グローバル opencode 設定（どのディレクトリでもプラグイン + OTEL 有効）
COPY --chmod=0644 opencode.jsonc /home/agent/.config/opencode/opencode.jsonc
