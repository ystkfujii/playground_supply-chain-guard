import express from "express";
import helmet from "helmet";
import axios from "axios";
import { v4 as uuidv4 } from "uuid";

const app = express();
const port = Number(process.env.PORT || 8080);
const startedAt = Date.now();

app.use(helmet());
app.use(express.json());

app.get("/", (_req, res) => {
  res.json({
    service: "kubescape-demo-node",
    message: "Hello from the Node.js demo app",
    requestId: uuidv4(),
    dependencies: {
      express: "enabled",
      helmet: "enabled",
      axios: axios.VERSION,
      uuid: "enabled"
    },
    upstream: process.env.UPSTREAM_URL || "not configured"
  });
});

app.get("/healthz", (_req, res) => {
  res.json({ status: "ok" });
});

app.get("/readyz", (_req, res) => {
  res.json({ status: "ready" });
});

app.get("/metadata", (_req, res) => {
  res.json({
    now: new Date().toISOString(),
    uptimeSeconds: Math.round((Date.now() - startedAt) / 1000),
    node: process.version
  });
});

app.listen(port, "0.0.0.0", () => {
  console.log(`kubescape-demo-node listening on :${port}`);
});
