#!/usr/bin/env tsx

import { spawn } from "child_process";

// Minimal test without SDK
const serverProcess = spawn("go", ["run", "../../examples/stdio_server/main.go"], {
  stdio: ["pipe", "pipe", "inherit"],
});

// Send initialize
const initRequest = {
  jsonrpc: "2.0",
  id: 1,
  method: "initialize",
  params: {
    protocolVersion: "2024-11-05",
    capabilities: {},
    clientInfo: {
      name: "test",
      version: "1.0"
    }
  }
};

console.log("Sending:", JSON.stringify(initRequest));
serverProcess.stdin.write(JSON.stringify(initRequest) + "\n");

// Read response
serverProcess.stdout.on("data", (data) => {
  console.log("Received:", data.toString());
});

setTimeout(() => {
  console.log("Closing...");
  serverProcess.kill();
  process.exit(0);
}, 3000);