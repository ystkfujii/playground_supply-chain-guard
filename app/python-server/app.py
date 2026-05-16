import os
import socket
import time
from datetime import datetime, timezone

import requests
from flask import Flask, jsonify

app = Flask(__name__)
STARTED_AT = time.time()


def dependency_probe() -> dict:
    """Return lightweight info that proves third-party deps are present."""
    return {
        "flask": "enabled",
        "requests": requests.__version__,
    }


@app.get("/")
def index():
    return jsonify(
        service="kubescape-demo-python",
        message="Hello from the Python demo app",
        hostname=socket.gethostname(),
        dependencies=dependency_probe(),
        upstream=os.getenv("UPSTREAM_URL", "not configured"),
    )


@app.get("/healthz")
def healthz():
    return jsonify(status="ok")


@app.get("/readyz")
def readyz():
    return jsonify(status="ready")


@app.get("/metadata")
def metadata():
    # This endpoint intentionally avoids calling external services during demos.
    return jsonify(
        now=datetime.now(timezone.utc).isoformat(),
        uptime_seconds=round(time.time() - STARTED_AT, 3),
        python=os.sys.version.split()[0],
    )


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=int(os.getenv("PORT", "8080")))
