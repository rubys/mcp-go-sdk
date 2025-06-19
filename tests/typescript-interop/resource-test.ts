#!/usr/bin/env tsx

import { spawn } from "child_process";
import path from "path";

async function main() {
  console.log("Testing resource read with specific parameters...");
  
  const serverDir = path.resolve(__dirname, "../..", "examples/stdio_server");
  const goProcess = spawn("go", ["run", "main.go"], {
    cwd: serverDir,
    stdio: ["pipe", "pipe", "inherit"],
  });

  console.log("Server started, PID:", goProcess.pid);

  // Wait for server to start
  await new Promise(resolve => setTimeout(resolve, 1000));

  let responseBuffer = "";
  goProcess.stdout!.on("data", (data) => {
    responseBuffer += data.toString();
    const lines = responseBuffer.split("\n");
    responseBuffer = lines.pop() || "";
    
    for (const line of lines) {
      if (line.trim()) {
        try {
          const response = JSON.parse(line);
          console.log("Response:", JSON.stringify(response, null, 2));
        } catch (e) {
          console.log("Non-JSON:", line);
        }
      }
    }
  });

  // Send initialize request
  const initRequest = {
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2024-11-05",
      capabilities: {},
      clientInfo: { name: "resource-test", version: "1.0.0" }
    }
  };
  console.log("Sending initialize...");
  goProcess.stdin!.write(JSON.stringify(initRequest) + "\n");

  // Wait a bit then send initialized notification
  setTimeout(() => {
    const initialized = { jsonrpc: "2.0", method: "initialized" };
    console.log("Sending initialized...");
    goProcess.stdin!.write(JSON.stringify(initialized) + "\n");

    // Now test resource read with proper params
    setTimeout(() => {
      const readRequest = {
        jsonrpc: "2.0",
        id: 2,
        method: "resources/read",
        params: {
          uri: "file://example.txt"
        }
      };
      console.log("Sending resources/read with params:", JSON.stringify(readRequest.params));
      goProcess.stdin!.write(JSON.stringify(readRequest) + "\n");

      // Test tool call
      setTimeout(() => {
        const toolRequest = {
          jsonrpc: "2.0",
          id: 3,
          method: "tools/call",
          params: {
            name: "echo",
            arguments: {
              text: "Hello from test!"
            }
          }
        };
        console.log("Sending tools/call with params:", JSON.stringify(toolRequest.params));
        goProcess.stdin!.write(JSON.stringify(toolRequest) + "\n");

        // Kill process after a delay
        setTimeout(() => {
          goProcess.kill();
        }, 1000);
      }, 200);
    }, 200);
  }, 500);

  goProcess.on("exit", (code) => {
    console.log(`Server exited with code: ${code}`);
  });
}

main().catch(console.error);