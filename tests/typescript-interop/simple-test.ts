import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import path from "path";

async function simpleTest() {
  console.log("Starting simple test...");
  
  const serverDir = path.resolve(__dirname, "../..", "examples/stdio_server");
  console.log(`Server directory: ${serverDir}`);
  
  const transport = new StdioClientTransport({
    command: "go",
    args: ["run", "main.go"],
    cwd: serverDir,
    stderr: "inherit",
  });

  const client = new Client(
    { name: "simple-test-client", version: "1.0.0" },
    { capabilities: {} }
  );

  try {
    console.log("Connecting to server...");
    await client.connect(transport);
    console.log("✅ Connected successfully");
    
    console.log("Testing resources list...");
    const resources = await client.listResources();
    console.log("✅ Resources list:", resources.resources.length);
    
  } catch (error) {
    console.error("❌ Error:", error);
  } finally {
    await client.close();
    console.log("Connection closed");
  }
}

simpleTest().catch(console.error);
EOF < /dev/null