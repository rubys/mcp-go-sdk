#!/usr/bin/env tsx

import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

async function testBasicCommunication() {
  // Create transport that will spawn the Go server
  const transport = new StdioClientTransport({
    command: './server',
    args: [],
    stderr: 'inherit', // Show server output
  });

  // Create MCP client
  const client = new Client(
    {
      name: 'simple-test-client',
      version: '1.0.0',
    },
    {
      capabilities: {},
    }
  );

  try {
    // Connect to server
    await client.connect(transport);
    console.log('✅ Connected to MCP server');

    // Test 1: List tools
    const tools = await client.listTools();
    console.log('🔧 Available tools:', tools.tools.map(t => t.name));

    // Test 2: Call get_time tool
    console.log('\n📅 Calling get_time tool...');
    const timeResult = await client.callTool({ name: 'get_time' });
    console.log('⏰ Result:', timeResult.content[0]);

    // Test 3: Call progress_tool without cancellation (let it complete)
    console.log('\n⏳ Calling progress_tool (will complete in 10 seconds)...');
    const progressResult = await client.callTool({ 
      name: 'progress_tool', 
      arguments: { progress_token: 'test-token' }
    });
    console.log('✅ Progress tool completed:', progressResult.content[0]);

  } catch (error) {
    console.error('❌ Error:', error);
  } finally {
    await client.close();
    console.log('🔌 Disconnected');
    process.exit(0);
  }
}

testBasicCommunication().catch(error => {
  console.error('❌ Fatal error:', error);
  process.exit(1);
});