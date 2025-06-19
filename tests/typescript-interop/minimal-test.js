const { Client } = require("@modelcontextprotocol/sdk/client/index.js");
const { StdioClientTransport } = require("@modelcontextprotocol/sdk/client/stdio.js");
const path = require("path");

async function minimalTest() {
  console.log("=== Minimal TypeScript Test ===");
  
  const serverDir = path.resolve(__dirname, "../..", "examples/stdio_server");
  
  const transport = new StdioClientTransport({
    command: "go",
    args: ["run", "main.go"],
    cwd: serverDir,
    stderr: "pipe", // Don't inherit stderr to avoid clutter
  });

  const client = new Client(
    { name: "minimal-test", version: "1.0.0" },
    { capabilities: {} }
  );

  try {
    // Test 1: Connection
    console.log("Test 1: Connecting...");
    await client.connect(transport);
    console.log("✅ Connection successful");

    // Test 2: List Resources
    console.log("Test 2: List Resources...");
    const resources = await client.listResources();
    console.log(`✅ Resources: ${resources.resources.length} found`);

    // Test 3: Read Resource
    console.log("Test 3: Read Resource...");
    try {
      const content = await client.readResource("file://example.txt");
      console.log(`✅ Resource read: ${content.contents.length} content items`);
    } catch (err) {
      console.log(`❌ Resource read failed: ${err.message}`);
    }

    // Test 4: List Tools
    console.log("Test 4: List Tools...");
    const tools = await client.listTools();
    console.log(`✅ Tools: ${tools.tools.length} found`);

    // Test 5: Call Tool (this is where the issue likely is)
    console.log("Test 5: Call Tool...");
    try {
      const result = await client.callTool({ name: "echo", arguments: { text: "Hello!" } });
      console.log(`✅ Tool call successful: ${JSON.stringify(result)}`);
    } catch (err) {
      console.log(`❌ Tool call failed: ${err.message}`);
    }

    await client.close();
    console.log("=== Test completed ===");

  } catch (error) {
    console.error("❌ Test failed:", error.message);
  }
}

// Set a timeout to prevent hanging
const timeout = setTimeout(() => {
  console.error("❌ Test timed out after 10 seconds");
  process.exit(1);
}, 10000);

minimalTest().then(() => {
  clearTimeout(timeout);
}).catch((err) => {
  clearTimeout(timeout);
  console.error("❌ Test error:", err.message);
});