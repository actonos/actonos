#!/bin/sh
set -eu

EMBEDDINGD_PID=""
ACTOND_PID=""

shutdown() {
    if [ -n "${ACTOND_PID}" ]; then
        kill -TERM "${ACTOND_PID}" 2>/dev/null || true
    fi
    if [ -n "${EMBEDDINGD_PID}" ]; then
        kill -TERM "${EMBEDDINGD_PID}" 2>/dev/null || true
    fi
}

trap shutdown INT TERM

if [ -x /usr/local/bin/embeddingd ] && [ -f "${ACTON_EMBEDDING_MODEL_DIR}/model.onnx" ]; then
    /usr/local/bin/embeddingd \
        --listen-addr="${ACTON_EMBEDDING_LISTEN_ADDR}" \
        --model-dir="${ACTON_EMBEDDING_MODEL_DIR}" \
        --onnxruntime-library="${ONNXRUNTIME_SHARED_LIBRARY_PATH}" &
    EMBEDDINGD_PID="$!"
fi

/usr/local/bin/actond "$@" &
ACTOND_PID="$!"
if wait "${ACTOND_PID}"; then
    STATUS=0
else
    STATUS="$?"
fi
shutdown
wait || true
exit "${STATUS}"
