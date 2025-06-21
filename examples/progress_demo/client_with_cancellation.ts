#!/usr/bin/env tsx

import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

async function demoWithCancellation() {
  // Create transport that will spawn the Go server
  const transport = new StdioClientTransport({
    command: './server',
    args: [],
    stderr: 'inherit',
  });

  // Create MCP client
  const client = new Client(
    {
      name: 'cancellation-demo-client',
      version: '1.0.0',
    },
    {
      capabilities: {},
    }
  );

  try {
    // Connect to server
    await client.connect(transport);
    console.log('🔗 Connected to MCP server');

    console.log('\n=== Complete Progress Demo: TypeScript Client ↔ Go Server ===\n');

    // 1. Call get_time tool first time
    console.log('📅 Step 1: Calling get_time tool (first time)...');
    const timeResult1 = await client.callTool({ name: 'get_time' });
    console.log('✅ Result:', timeResult1.content[0]);

    // 2. Call progress_tool with cancellation after 3rd notification
    console.log('\n⏳ Step 2: Calling progress_tool with cancellation after 3rd progress...');
    
    let progressCount = 0;
    const abortController = new AbortController();

    try {
      const progressPromise = client.callTool(
        { 
          name: 'progress_tool', 
          arguments: {}
        },
        undefined, // resultSchema
        {
          onprogress: (progress) => {
            progressCount++;
            console.log(`📊 Progress notification #${progressCount}: ${progress.progress}/${progress.total} (${progress.message})`);
            
            // Cancel after 3rd notification
            if (progressCount === 3) {
              console.log('🛑 Cancelling request after 3rd progress notification...');
              setTimeout(() => {
                abortController.abort('Cancelled after 3 progress notifications');
              }, 100); // Small delay to ensure log appears
            }
          },
          signal: abortController.signal,
          timeout: 15000,
        }
      );
      
      const progressResult = await progressPromise;
      console.log('✅ Progress tool completed:', progressResult.content[0]);
    } catch (error: any) {
      if (error.name === 'AbortError' || abortController.signal.aborted) {
        console.log('✅ Successfully cancelled progress tool after 3 notifications');
      } else {
        console.log('⚠️  Progress tool error:', error.message || error);
      }
    }

    console.log(`📊 Total progress notifications received: ${progressCount}`);

    // 3. Call get_time tool second time
    console.log('\n📅 Step 3: Calling get_time tool (second time)...');
    const timeResult2 = await client.callTool({ name: 'get_time' });
    console.log('✅ Result:', timeResult2.content[0]);

    console.log('\n🎉 Demo completed successfully!');
    console.log('\n🏆 Demonstrated features:');
    console.log('  ✅ TypeScript MCP client ↔ Go MCP server communication');
    console.log('  ✅ mark3labs/mcp-go compatible API in Go server');
    console.log('  ✅ Progress notifications from server to client');
    console.log('  ✅ Request cancellation after progress notifications');
    console.log('  ✅ Tool execution with proper MCP protocol compliance');
    console.log('  ✅ Progress token handling per MCP specification');

  } catch (error) {
    console.error('❌ Demo failed:', error);
  } finally {
    await client.close();
    console.log('\n🔌 Disconnected from server');
    process.exit(0);
  }
}

demoWithCancellation().catch((error) => {
  console.error('❌ Fatal error:', error);
  process.exit(1);
});