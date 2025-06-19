#!/usr/bin/env tsx

import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { spawn } from "child_process";
import { promises as fs } from "fs";
import path from "path";

// Test configuration
const GO_SERVER_PATH = "../../examples/stdio_server/main.go";
const CONCURRENT_REQUESTS = 20;
const TEST_TIMEOUT = 60000; // 60 seconds

// ANSI colors for output
const colors = {
  green: "\x1b[32m",
  red: "\x1b[31m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  reset: "\x1b[0m",
};

// Test result type
interface TestResult {
  name: string;
  passed: boolean;
  duration: number;
  error?: string;
}

class GoServerTest {
  private client: Client | null = null;
  private results: TestResult[] = [];

  async setup(): Promise<void> {
    console.log(`${colors.blue}Starting Go MCP server...${colors.reset}`);
    
    // Create transport that will spawn the Go server
    const serverDir = path.resolve(__dirname, "../..", "examples/stdio_server");
    console.log(`Server directory: ${serverDir}`);
    
    const transport = new StdioClientTransport({
      command: "go",
      args: ["run", "main.go"],
      cwd: serverDir,
      stderr: "inherit", // Show server errors
    });

    this.client = new Client(
      {
        name: "typescript-test-client",
        version: "1.0.0",
      },
      {
        capabilities: {},
      }
    );

    // Connect to the server
    try {
      await this.client.connect(transport);
      console.log(`${colors.green}✓ Connected to Go MCP server${colors.reset}`);
    } catch (error) {
      console.error(`${colors.red}Failed to connect: ${error}${colors.reset}`);
      throw error;
    }
  }

  async teardown(): Promise<void> {
    if (this.client) {
      await this.client.close();
    }
  }

  async runTest(name: string, testFn: () => Promise<void>): Promise<void> {
    const startTime = Date.now();
    try {
      await testFn();
      const duration = Date.now() - startTime;
      this.results.push({ name, passed: true, duration });
      console.log(
        `${colors.green}✓${colors.reset} ${name} (${duration}ms)`
      );
    } catch (error) {
      const duration = Date.now() - startTime;
      const errorMsg = error instanceof Error ? error.message : String(error);
      this.results.push({ name, passed: false, duration, error: errorMsg });
      console.log(
        `${colors.red}✗${colors.reset} ${name} (${duration}ms)\n  ${colors.red}${errorMsg}${colors.reset}`
      );
    }
  }

  async testBasicConnection(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    // Test basic functionality by listing resources - this confirms connection works
    const resources = await this.client.listResources();
    if (!resources) throw new Error("Failed to list resources");
    
    // The mere fact that we can make this call means the connection is working properly
    console.log(`  ${colors.blue}Connection verified via resources list call${colors.reset}`);
  }

  async testListResources(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    const resources = await this.client.listResources();
    if (!resources || resources.resources.length === 0) {
      throw new Error("No resources found");
    }
    
    const exampleResource = resources.resources.find(
      (r) => r.uri === "file://example.txt"
    );
    if (!exampleResource) {
      throw new Error("Expected resource 'file://example.txt' not found");
    }
  }

  async testReadResource(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    const result = await this.client.readResource("file://example.txt");
    if (!result || !result.contents || result.contents.length === 0) {
      throw new Error("No content returned from resource");
    }
    
    const content = result.contents[0];
    if (content.type !== "text") {
      throw new Error(`Unexpected content type: ${content.type}`);
    }
    
    if (!content.text.includes("concurrent Go MCP server")) {
      throw new Error("Unexpected content text");
    }
  }

  async testListTools(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    const tools = await this.client.listTools();
    if (!tools || tools.tools.length === 0) {
      throw new Error("No tools found");
    }
    
    const echoTool = tools.tools.find((t) => t.name === "echo");
    if (!echoTool) {
      throw new Error("Expected tool 'echo' not found");
    }
  }

  async testCallTool(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    const result = await this.client.callTool({
      name: "echo",
      arguments: { text: "Hello from TypeScript!" },
    });
    
    if (!result || !result.content || result.content.length === 0) {
      throw new Error("No content returned from tool");
    }
    
    const content = result.content[0];
    if (content.type !== "text") {
      throw new Error(`Unexpected content type: ${content.type}`);
    }
    
    if (!content.text.includes("Echo: Hello from TypeScript!")) {
      throw new Error(`Unexpected echo response: ${content.text}`);
    }
  }

  async testListPrompts(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    const prompts = await this.client.listPrompts();
    if (!prompts || prompts.prompts.length === 0) {
      throw new Error("No prompts found");
    }
    
    const greetingPrompt = prompts.prompts.find((p) => p.name === "greeting");
    if (!greetingPrompt) {
      throw new Error("Expected prompt 'greeting' not found");
    }
  }

  async testGetPrompt(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    const result = await this.client.getPrompt({
      name: "greeting",
      arguments: { name: "TypeScript User" },
    });
    
    if (!result || !result.messages || result.messages.length === 0) {
      throw new Error("No messages returned from prompt");
    }
    
    const message = result.messages[0];
    if (message.role !== "user") {
      throw new Error(`Unexpected message role: ${message.role}`);
    }
    
    const content = message.content;
    if (content.type !== "text") {
      throw new Error(`Unexpected content type: ${content.type}`);
    }
    
    if (!content.text.includes("Hello, TypeScript User!")) {
      throw new Error(`Unexpected greeting: ${content.text}`);
    }
  }

  async testConcurrentRequests(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    console.log(
      `  ${colors.yellow}Running ${CONCURRENT_REQUESTS} concurrent requests...${colors.reset}`
    );
    
    const startTime = Date.now();
    const promises: Promise<any>[] = [];
    
    // Mix of different request types
    for (let i = 0; i < CONCURRENT_REQUESTS; i++) {
      switch (i % 4) {
        case 0:
          promises.push(this.client.listResources());
          break;
        case 1:
          promises.push(this.client.readResource("file://example.txt"));
          break;
        case 2:
          promises.push(
            this.client.callTool({ name: "echo", arguments: { text: `Concurrent ${i}` } })
          );
          break;
        case 3:
          promises.push(
            this.client.getPrompt({ name: "greeting", arguments: { name: `User${i}` } })
          );
          break;
      }
    }
    
    // Wait for all requests to complete
    const results = await Promise.all(promises);
    const duration = Date.now() - startTime;
    
    // Verify all requests succeeded
    if (results.length !== CONCURRENT_REQUESTS) {
      throw new Error(
        `Expected ${CONCURRENT_REQUESTS} results, got ${results.length}`
      );
    }
    
    console.log(
      `  ${colors.green}Completed ${CONCURRENT_REQUESTS} concurrent requests in ${duration}ms${colors.reset}`
    );
    console.log(
      `  ${colors.blue}Average time per request: ${(
        duration / CONCURRENT_REQUESTS
      ).toFixed(2)}ms${colors.reset}`
    );
  }

  async testHighLoadConcurrency(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    const HIGH_LOAD = 100;
    console.log(
      `  ${colors.yellow}Running high-load test with ${HIGH_LOAD} requests...${colors.reset}`
    );
    
    const startTime = Date.now();
    const promises: Promise<any>[] = [];
    
    // All requests of the same type to stress test a single endpoint
    for (let i = 0; i < HIGH_LOAD; i++) {
      promises.push(
        this.client.callTool({ name: "echo", arguments: { text: `High load ${i}` } })
      );
    }
    
    try {
      const results = await Promise.all(promises);
      const duration = Date.now() - startTime;
      
      console.log(
        `  ${colors.green}Completed ${HIGH_LOAD} requests in ${duration}ms${colors.reset}`
      );
      console.log(
        `  ${colors.blue}Throughput: ${(
          (HIGH_LOAD / duration) * 1000
        ).toFixed(2)} requests/second${colors.reset}`
      );
    } catch (error) {
      throw new Error(`High load test failed: ${error}`);
    }
  }

  async runAllTests(): Promise<void> {
    console.log(
      `\n${colors.blue}=== TypeScript Client → Go Server Interoperability Tests ===${colors.reset}\n`
    );
    
    try {
      await this.setup();
      
      // Run all tests
      await this.runTest("Basic Connection", () => this.testBasicConnection());
      await this.runTest("List Resources", () => this.testListResources());
      await this.runTest("Read Resource", () => this.testReadResource());
      await this.runTest("List Tools", () => this.testListTools());
      await this.runTest("Call Tool", () => this.testCallTool());
      await this.runTest("List Prompts", () => this.testListPrompts());
      await this.runTest("Get Prompt", () => this.testGetPrompt());
      await this.runTest("Concurrent Requests", () =>
        this.testConcurrentRequests()
      );
      await this.runTest("High Load Concurrency", () =>
        this.testHighLoadConcurrency()
      );
      
      // Print summary
      this.printSummary();
    } finally {
      await this.teardown();
    }
  }

  private printSummary(): void {
    console.log(`\n${colors.blue}=== Test Summary ===${colors.reset}\n`);
    
    const passed = this.results.filter((r) => r.passed).length;
    const failed = this.results.filter((r) => !r.passed).length;
    const totalDuration = this.results.reduce((sum, r) => sum + r.duration, 0);
    
    console.log(`Total tests: ${this.results.length}`);
    console.log(`${colors.green}Passed: ${passed}${colors.reset}`);
    console.log(`${colors.red}Failed: ${failed}${colors.reset}`);
    console.log(`Total duration: ${totalDuration}ms`);
    
    if (failed === 0) {
      console.log(
        `\n${colors.green}✅ All tests passed! The Go MCP SDK is fully compatible with TypeScript clients.${colors.reset}`
      );
    } else {
      console.log(`\n${colors.red}❌ Some tests failed.${colors.reset}`);
      process.exit(1);
    }
  }
}

// Run the tests
async function main() {
  const tester = new GoServerTest();
  
  try {
    await tester.runAllTests();
  } catch (error) {
    console.error(`${colors.red}Test suite error: ${error}${colors.reset}`);
    process.exit(1);
  }
  
  // Ensure process exits
  process.exit(0);
}

// Handle uncaught errors
process.on("unhandledRejection", (error) => {
  console.error(`${colors.red}Unhandled rejection: ${error}${colors.reset}`);
  process.exit(1);
});

// Run with timeout
const timeout = setTimeout(() => {
  console.error(
    `${colors.red}Test suite timeout after ${TEST_TIMEOUT}ms${colors.reset}`
  );
  process.exit(1);
}, TEST_TIMEOUT);

main().then(() => {
  clearTimeout(timeout);
});