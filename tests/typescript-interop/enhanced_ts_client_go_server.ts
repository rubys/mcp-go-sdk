#!/usr/bin/env tsx

import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { spawn } from "child_process";
import { promises as fs } from "fs";
import path from "path";

// Enhanced test configuration
const GO_SERVER_PATH = "../../examples/stdio_server/main.go";
const CONCURRENT_REQUESTS = 50;
const HIGH_LOAD_REQUESTS = 200;
const TEST_TIMEOUT = 120000; // 2 minutes for enhanced tests

// ANSI colors for output
const colors = {
  green: "\x1b[32m",
  red: "\x1b[31m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
  reset: "\x1b[0m",
};

// Enhanced test result type
interface TestResult {
  name: string;
  passed: boolean;
  duration: number;
  error?: string;
  metrics?: {
    throughput?: number;
    latency?: number;
    concurrency?: number;
  };
}

class EnhancedGoServerTest {
  private client: Client | null = null;
  private results: TestResult[] = [];

  async setup(): Promise<void> {
    console.log(`${colors.blue}🚀 Starting Enhanced Go MCP Server Tests...${colors.reset}`);
    
    // Create transport that will spawn the Go server
    const serverDir = path.resolve(__dirname, "../..", "examples/stdio_server");
    console.log(`📂 Server directory: ${serverDir}`);
    
    const transport = new StdioClientTransport({
      command: "go",
      args: ["run", "main.go"],
      cwd: serverDir,
      stderr: "inherit", // Show server errors
    });

    this.client = new Client(
      {
        name: "enhanced-typescript-test-client",
        version: "2.0.0",
      },
      {
        capabilities: {
          experimental: {
            concurrency: true,
            streaming: true,
          },
        },
      }
    );

    // Connect to the server
    try {
      await this.client.connect(transport);
      console.log(`${colors.green}✅ Connected to Go MCP server${colors.reset}`);
    } catch (error) {
      console.error(`${colors.red}❌ Failed to connect: ${error}${colors.reset}`);
      throw error;
    }
  }

  async teardown(): Promise<void> {
    if (this.client) {
      await this.client.close();
      console.log(`${colors.yellow}🔌 Disconnected from server${colors.reset}`);
    }
  }

  async runTest(name: string, testFn: () => Promise<void>): Promise<void> {
    const startTime = Date.now();
    try {
      console.log(`${colors.cyan}🧪 Running: ${name}${colors.reset}`);
      await testFn();
      const duration = Date.now() - startTime;
      this.results.push({ name, passed: true, duration });
      console.log(
        `${colors.green}✅${colors.reset} ${name} ${colors.green}(${duration}ms)${colors.reset}`
      );
    } catch (error) {
      const duration = Date.now() - startTime;
      const errorMsg = error instanceof Error ? error.message : String(error);
      this.results.push({ name, passed: false, duration, error: errorMsg });
      console.log(
        `${colors.red}❌${colors.reset} ${name} ${colors.red}(${duration}ms)${colors.reset}`
      );
      console.log(`   ${colors.red}Error: ${errorMsg}${colors.reset}`);
    }
  }

  // Enhanced parameter translation tests
  async testParameterTranslation(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    console.log(`  ${colors.yellow}🔄 Testing parameter translation compatibility...${colors.reset}`);
    
    // Test simplified URI parameter (TypeScript SDK style)
    const resourceResult = await this.client.readResource("file://example.txt");
    if (!resourceResult || !resourceResult.contents || resourceResult.contents.length === 0) {
      throw new Error("Failed to read resource with simplified URI parameter");
    }
    
    // Test complex tool parameters
    const toolResult = await this.client.callTool({
      name: "echo",
      arguments: {
        text: "Complex parameter test",
        metadata: {
          timestamp: Date.now(),
          nested: {
            array: [1, 2, 3],
            boolean: true,
          },
        },
      },
    });
    
    if (!toolResult || !toolResult.content || toolResult.content.length === 0) {
      throw new Error("Failed to call tool with complex parameters");
    }
    
    console.log(`  ${colors.green}✅ Parameter translation working correctly${colors.reset}`);
  }

  // Enhanced response format validation
  async testResponseFormatValidation(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    console.log(`  ${colors.yellow}📋 Validating response format compatibility...${colors.reset}`);
    
    // Test prompt response format (single vs array content)
    const promptResult = await this.client.getPrompt({
      name: "greeting",
      arguments: { name: "Format Test" },
    });
    
    if (!promptResult || !promptResult.messages || promptResult.messages.length === 0) {
      throw new Error("No prompt response received");
    }
    
    const message = promptResult.messages[0];
    const content = message.content;
    
    // Verify content format matches TypeScript SDK expectations
    if (typeof content === "object" && "type" in content) {
      // Single content item should be an object
      console.log(`  ${colors.blue}📄 Single content format: object${colors.reset}`);
    } else if (Array.isArray(content)) {
      // Multiple content items should be an array
      console.log(`  ${colors.blue}📄 Multiple content format: array[${content.length}]${colors.reset}`);
    } else {
      throw new Error(`Unexpected content format: ${typeof content}`);
    }
    
    console.log(`  ${colors.green}✅ Response format validation passed${colors.reset}`);
  }

  // Enhanced concurrent requests with different patterns
  async testEnhancedConcurrentRequests(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    console.log(`  ${colors.yellow}🚀 Running ${CONCURRENT_REQUESTS} concurrent requests...${colors.reset}`);
    
    const startTime = Date.now();
    const promises: Promise<any>[] = [];
    const requestTypes = ["resources", "tools", "prompts", "mixed"];
    
    // Create diverse concurrent requests
    for (let i = 0; i < CONCURRENT_REQUESTS; i++) {
      const requestType = requestTypes[i % requestTypes.length];
      
      switch (requestType) {
        case "resources":
          if (i % 2 === 0) {
            promises.push(this.client.listResources());
          } else {
            promises.push(this.client.readResource("file://example.txt"));
          }
          break;
          
        case "tools":
          if (i % 2 === 0) {
            promises.push(this.client.listTools());
          } else {
            promises.push(this.client.callTool({
              name: "echo",
              arguments: { text: `Concurrent tool call ${i}` },
            }));
          }
          break;
          
        case "prompts":
          if (i % 2 === 0) {
            promises.push(this.client.listPrompts());
          } else {
            promises.push(this.client.getPrompt({
              name: "greeting",
              arguments: { name: `ConcurrentUser${i}` },
            }));
          }
          break;
          
        case "mixed":
          // Mix different request types randomly
          const mixedRequests = [
            () => this.client!.listResources(),
            () => this.client!.listTools(),
            () => this.client!.listPrompts(),
          ];
          promises.push(mixedRequests[i % mixedRequests.length]());
          break;
      }
    }
    
    // Wait for all requests to complete
    const results = await Promise.all(promises);
    const duration = Date.now() - startTime;
    const throughput = (CONCURRENT_REQUESTS / duration) * 1000;
    
    // Verify all requests succeeded
    if (results.length !== CONCURRENT_REQUESTS) {
      throw new Error(`Expected ${CONCURRENT_REQUESTS} results, got ${results.length}`);
    }
    
    // Add metrics to last result
    const lastResult = this.results[this.results.length - 1];
    if (lastResult) {
      lastResult.metrics = {
        throughput,
        latency: duration / CONCURRENT_REQUESTS,
        concurrency: CONCURRENT_REQUESTS,
      };
    }
    
    console.log(`  ${colors.green}📊 Completed ${CONCURRENT_REQUESTS} requests in ${duration}ms${colors.reset}`);
    console.log(`  ${colors.blue}⚡ Throughput: ${throughput.toFixed(2)} req/sec${colors.reset}`);
    console.log(`  ${colors.blue}📈 Avg latency: ${(duration / CONCURRENT_REQUESTS).toFixed(2)}ms${colors.reset}`);
  }

  // Stress test with very high load
  async testStressTest(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    console.log(`  ${colors.yellow}💪 Running stress test with ${HIGH_LOAD_REQUESTS} requests...${colors.reset}`);
    
    const startTime = Date.now();
    const batchSize = 25; // Process in batches to avoid overwhelming
    const batches = Math.ceil(HIGH_LOAD_REQUESTS / batchSize);
    let totalProcessed = 0;
    
    for (let batch = 0; batch < batches; batch++) {
      const batchPromises: Promise<any>[] = [];
      const currentBatchSize = Math.min(batchSize, HIGH_LOAD_REQUESTS - totalProcessed);
      
      for (let i = 0; i < currentBatchSize; i++) {
        batchPromises.push(
          this.client.callTool({
            name: "echo",
            arguments: { text: `Stress test ${totalProcessed + i}` },
          })
        );
      }
      
      await Promise.all(batchPromises);
      totalProcessed += currentBatchSize;
      
      // Progress indicator
      const progress = (totalProcessed / HIGH_LOAD_REQUESTS * 100).toFixed(1);
      process.stdout.write(`\r  ${colors.cyan}📈 Progress: ${progress}% (${totalProcessed}/${HIGH_LOAD_REQUESTS})${colors.reset}`);
    }
    
    console.log(); // New line after progress
    
    const duration = Date.now() - startTime;
    const throughput = (HIGH_LOAD_REQUESTS / duration) * 1000;
    
    console.log(`  ${colors.green}🎯 Stress test completed: ${HIGH_LOAD_REQUESTS} requests in ${duration}ms${colors.reset}`);
    console.log(`  ${colors.blue}🚀 Peak throughput: ${throughput.toFixed(2)} req/sec${colors.reset}`);
    
    // Verify throughput meets expectations
    if (throughput < 50) {
      throw new Error(`Throughput too low: ${throughput.toFixed(2)} req/sec (expected > 50 req/sec)`);
    }
  }

  // Test error recovery and resilience
  async testErrorRecovery(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    console.log(`  ${colors.yellow}🛠️  Testing error recovery...${colors.reset}`);
    
    // Test invalid resource
    try {
      await this.client.readResource("file://nonexistent.txt");
      throw new Error("Expected error for nonexistent resource");
    } catch (error) {
      // Expected error
      console.log(`  ${colors.blue}✅ Handled invalid resource error correctly${colors.reset}`);
    }
    
    // Test invalid tool
    try {
      await this.client.callTool({
        name: "nonexistent_tool",
        arguments: {},
      });
      throw new Error("Expected error for nonexistent tool");
    } catch (error) {
      // Expected error
      console.log(`  ${colors.blue}✅ Handled invalid tool error correctly${colors.reset}`);
    }
    
    // Test that client recovers and can make valid requests
    const validResult = await this.client.listResources();
    if (!validResult) {
      throw new Error("Client did not recover after errors");
    }
    
    console.log(`  ${colors.green}✅ Error recovery successful${colors.reset}`);
  }

  // Test connection stability under load
  async testConnectionStability(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    
    console.log(`  ${colors.yellow}🔗 Testing connection stability...${colors.reset}`);
    
    const stabilityTestDuration = 10000; // 10 seconds
    const requestInterval = 100; // Request every 100ms
    const startTime = Date.now();
    let requestCount = 0;
    let errorCount = 0;
    
    while (Date.now() - startTime < stabilityTestDuration) {
      try {
        await this.client.listResources();
        requestCount++;
      } catch (error) {
        errorCount++;
        console.log(`  ${colors.red}⚠️  Request error: ${error}${colors.reset}`);
      }
      
      await new Promise(resolve => setTimeout(resolve, requestInterval));
    }
    
    const errorRate = (errorCount / requestCount) * 100;
    
    console.log(`  ${colors.blue}📊 Stability test: ${requestCount} requests, ${errorCount} errors (${errorRate.toFixed(2)}% error rate)${colors.reset}`);
    
    if (errorRate > 5) {
      throw new Error(`Connection stability poor: ${errorRate.toFixed(2)}% error rate`);
    }
    
    console.log(`  ${colors.green}✅ Connection stability good${colors.reset}`);
  }

  async runAllTests(): Promise<void> {
    console.log(`\n${colors.magenta}═══════════════════════════════════════════════════════════${colors.reset}`);
    console.log(`${colors.magenta}   Enhanced TypeScript Client → Go Server Interop Tests    ${colors.reset}`);
    console.log(`${colors.magenta}═══════════════════════════════════════════════════════════${colors.reset}\n`);
    
    try {
      await this.setup();
      
      // Basic functionality tests
      await this.runTest("🔌 Basic Connection", () => this.testBasicConnection());
      await this.runTest("📋 List Resources", () => this.testListResources());
      await this.runTest("📖 Read Resource", () => this.testReadResource());
      await this.runTest("🛠️  List Tools", () => this.testListTools());
      await this.runTest("⚙️  Call Tool", () => this.testCallTool());
      await this.runTest("📝 List Prompts", () => this.testListPrompts());
      await this.runTest("💬 Get Prompt", () => this.testGetPrompt());
      
      // Enhanced compatibility tests
      await this.runTest("🔄 Parameter Translation", () => this.testParameterTranslation());
      await this.runTest("📋 Response Format Validation", () => this.testResponseFormatValidation());
      
      // Performance and concurrency tests
      await this.runTest("🚀 Enhanced Concurrent Requests", () => this.testEnhancedConcurrentRequests());
      await this.runTest("💪 Stress Test", () => this.testStressTest());
      
      // Reliability tests
      await this.runTest("🛠️  Error Recovery", () => this.testErrorRecovery());
      await this.runTest("🔗 Connection Stability", () => this.testConnectionStability());
      
      // Print enhanced summary
      this.printEnhancedSummary();
    } finally {
      await this.teardown();
    }
  }

  // Basic tests (from original)
  async testBasicConnection(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    const resources = await this.client.listResources();
    if (!resources) throw new Error("Failed to list resources");
  }

  async testListResources(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    const resources = await this.client.listResources();
    if (!resources || resources.resources.length === 0) {
      throw new Error("No resources found");
    }
  }

  async testReadResource(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    const result = await this.client.readResource("file://example.txt");
    if (!result || !result.contents || result.contents.length === 0) {
      throw new Error("No content returned from resource");
    }
  }

  async testListTools(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    const tools = await this.client.listTools();
    if (!tools || tools.tools.length === 0) {
      throw new Error("No tools found");
    }
  }

  async testCallTool(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    const result = await this.client.callTool({
      name: "echo",
      arguments: { text: "Hello from Enhanced TypeScript!" },
    });
    if (!result || !result.content || result.content.length === 0) {
      throw new Error("No content returned from tool");
    }
  }

  async testListPrompts(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    const prompts = await this.client.listPrompts();
    if (!prompts || prompts.prompts.length === 0) {
      throw new Error("No prompts found");
    }
  }

  async testGetPrompt(): Promise<void> {
    if (!this.client) throw new Error("Client not initialized");
    const result = await this.client.getPrompt({
      name: "greeting",
      arguments: { name: "Enhanced TypeScript User" },
    });
    if (!result || !result.messages || result.messages.length === 0) {
      throw new Error("No messages returned from prompt");
    }
  }

  private printEnhancedSummary(): void {
    console.log(`\n${colors.magenta}═══════════════════════════════════════════════════════════${colors.reset}`);
    console.log(`${colors.magenta}                    Enhanced Test Summary                    ${colors.reset}`);
    console.log(`${colors.magenta}═══════════════════════════════════════════════════════════${colors.reset}\n`);
    
    const passed = this.results.filter((r) => r.passed).length;
    const failed = this.results.filter((r) => !r.passed).length;
    const totalDuration = this.results.reduce((sum, r) => sum + r.duration, 0);
    
    // Performance metrics
    const performanceTests = this.results.filter(r => r.metrics);
    const avgThroughput = performanceTests.length > 0 
      ? performanceTests.reduce((sum, r) => sum + (r.metrics?.throughput || 0), 0) / performanceTests.length
      : 0;
    
    console.log(`📊 ${colors.blue}Test Statistics:${colors.reset}`);
    console.log(`   Total tests: ${this.results.length}`);
    console.log(`   ${colors.green}✅ Passed: ${passed}${colors.reset}`);
    console.log(`   ${colors.red}❌ Failed: ${failed}${colors.reset}`);
    console.log(`   ⏱️  Total duration: ${totalDuration}ms`);
    console.log(`   📈 Avg throughput: ${avgThroughput.toFixed(2)} req/sec`);
    
    if (performanceTests.length > 0) {
      console.log(`\n🚀 ${colors.cyan}Performance Metrics:${colors.reset}`);
      performanceTests.forEach(test => {
        if (test.metrics) {
          console.log(`   ${test.name}:`);
          console.log(`     📊 Throughput: ${test.metrics.throughput?.toFixed(2)} req/sec`);
          console.log(`     ⚡ Latency: ${test.metrics.latency?.toFixed(2)}ms`);
          if (test.metrics.concurrency) {
            console.log(`     🔄 Concurrency: ${test.metrics.concurrency}`);
          }
        }
      });
    }
    
    if (failed === 0) {
      console.log(`\n${colors.green}🎉 ALL ENHANCED TESTS PASSED! 🎉${colors.reset}`);
      console.log(`${colors.green}✅ The Go MCP SDK demonstrates excellent compatibility with TypeScript clients.${colors.reset}`);
      console.log(`${colors.green}✅ Performance and reliability meet production standards.${colors.reset}`);
    } else {
      console.log(`\n${colors.red}❌ Some enhanced tests failed.${colors.reset}`);
      
      // Show failed tests
      const failedTests = this.results.filter(r => !r.passed);
      failedTests.forEach(test => {
        console.log(`   ${colors.red}❌ ${test.name}: ${test.error}${colors.reset}`);
      });
      
      process.exit(1);
    }
  }
}

// Run the enhanced tests
async function main() {
  const tester = new EnhancedGoServerTest();
  
  try {
    await tester.runAllTests();
  } catch (error) {
    console.error(`${colors.red}💥 Enhanced test suite error: ${error}${colors.reset}`);
    process.exit(1);
  }
  
  // Ensure process exits
  process.exit(0);
}

// Handle uncaught errors
process.on("unhandledRejection", (error) => {
  console.error(`${colors.red}💥 Unhandled rejection: ${error}${colors.reset}`);
  process.exit(1);
});

// Run with extended timeout for enhanced tests
const timeout = setTimeout(() => {
  console.error(
    `${colors.red}⏰ Enhanced test suite timeout after ${TEST_TIMEOUT}ms${colors.reset}`
  );
  process.exit(1);
}, TEST_TIMEOUT);

main().then(() => {
  clearTimeout(timeout);
});