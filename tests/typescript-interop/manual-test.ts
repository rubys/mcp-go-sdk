#!/usr/bin/env tsx

import { spawn } from "child_process";
import path from "path";

async function main() {
  console.log("Starting Go server manually...");
  
  const serverDir = path.resolve(__dirname, "../..", "examples/stdio_server");
  const goProcess = spawn("go", ["run", "main.go"], {
    cwd: serverDir,
    stdio: ["pipe", "pipe", "inherit"], // stdin, stdout, stderr
  });

  console.log("Server started, PID:", goProcess.pid);

  // Wait a moment for server to start
  await new Promise(resolve => setTimeout(resolve, 1000));

  // Send initialize request
  const initRequest = {
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2024-11-05",
      capabilities: {},
      clientInfo: {
        name: "manual-test-client",
        version: "1.0.0"
      }
    }
  };

  console.log("Sending initialize request:", JSON.stringify(initRequest));
  goProcess.stdin!.write(JSON.stringify(initRequest) + "\n");

  let initResponseReceived = false;

  // Listen for responses
  let responseBuffer = "";
  goProcess.stdout!.on("data", (data) => {
    responseBuffer += data.toString();
    console.log("Received data:", data.toString());
    
    // Try to parse JSON lines
    const lines = responseBuffer.split("\n");
    responseBuffer = lines.pop() || ""; // Keep partial line in buffer
    
    for (const line of lines) {
      if (line.trim()) {
        try {
          const response = JSON.parse(line);
          console.log("Parsed response:", JSON.stringify(response, null, 2));
          
          // If this is the initialize response, send initialized notification
          if (response.id === 1 && response.result && !initResponseReceived) {
            initResponseReceived = true;
            console.log("Sending initialized notification...");
            const initializedNotification = {
              jsonrpc: "2.0",
              method: "initialized"
            };
            goProcess.stdin!.write(JSON.stringify(initializedNotification) + "\n");
            
            // Now test a basic operation
            setTimeout(() => {
              console.log("Sending resources/list request...");
              const listRequest = {
                jsonrpc: "2.0",
                id: 2,
                method: "resources/list"
              };
              goProcess.stdin!.write(JSON.stringify(listRequest) + "\n");
            }, 100);
          }
        } catch (e) {
          console.log("Non-JSON line:", line);
        }
      }
    }
  });

  goProcess.stdout!.on("end", () => {
    console.log("Server stdout ended");
  });

  goProcess.on("exit", (code) => {
    console.log(`Server exited with code: ${code}`);
  });

  // Wait for response or timeout
  await new Promise((resolve) => {
    setTimeout(() => {
      console.log("Timeout reached, killing server");
      goProcess.kill();
      resolve(null);
    }, 5000);
  });
}

main().catch(console.error);