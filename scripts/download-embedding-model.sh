#!/usr/bin/env bash
set -euo pipefail

REVISION="614241f622f53c4eeff9890bdc4f31cfecc418b3"
DESTINATION="${1:-./build/models/multilingual-e5-small/${REVISION}}"
BASE_URL="https://huggingface.co/intfloat/multilingual-e5-small/resolve/${REVISION}/onnx"

mkdir -p "${DESTINATION}"

download() {
    local name="$1"
    local sha256="$2"
    local target="${DESTINATION}/${name}"
    if [ -f "${target}" ] && [ -n "${sha256}" ]; then
        local existing
        existing="$(sha256sum "${target}" | awk '{print $1}')"
        if [ "${existing}" = "${sha256}" ]; then
            return
        fi
    fi
    curl --fail --location --retry 3 --output "${target}.part" "${BASE_URL}/${name}"
    if [ -n "${sha256}" ]; then
        echo "${sha256}  ${target}.part" | sha256sum --check --status
    fi
    mv "${target}.part" "${target}"
}

download "model.onnx" "ca456c06b3a9505ddfd9131408916dd79290368331e7d76bb621f1cba6bc8665"
download "tokenizer.json" "0b44a9d7b51c3c62626640cda0e2c2f70fdacdc25bbbd68038369d14ebdf4c39"
download "config.json" ""
download "tokenizer_config.json" ""
download "special_tokens_map.json" ""

cat > "${DESTINATION}/manifest.json" <<EOF
{
  "model_id": "intfloat/multilingual-e5-small",
  "revision": "${REVISION}",
  "dimension": 384,
  "max_tokens": 512,
  "model_sha256": "ca456c06b3a9505ddfd9131408916dd79290368331e7d76bb621f1cba6bc8665",
  "tokenizer_sha256": "0b44a9d7b51c3c62626640cda0e2c2f70fdacdc25bbbd68038369d14ebdf4c39"
}
EOF
