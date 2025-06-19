import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListResourcesRequestSchema,
  ListToolsRequestSchema,
  ListPromptsRequestSchema,
  ReadResourceRequestSchema,
  GetPromptRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

// Create server instance
const server = new Server(
  {
    name: "test-typescript-server",
    version: "1.0.0",
  },
  {
    capabilities: {
      resources: {},
      tools: {},
      prompts: {},
    },
  }
);

// Register resource handlers
server.setRequestHandler(ListResourcesRequestSchema, async () => {
  return {
    resources: [
      {
        uri: "file://example.txt",
        name: "Example Text File",
        description: "An example text resource from TypeScript server",
      },
    ],
  };
});

server.setRequestHandler(ReadResourceRequestSchema, async (request) => {
  console.error(`[TS Server] ReadResource called with:`, JSON.stringify(request.params));
  const { uri } = request.params;
  
  if (uri === "file://example.txt") {
    return {
      contents: [
        {
          uri: "file://example.txt",
          mimeType: "text/plain",
          text: "This is example content from the TypeScript MCP server!",
        },
      ],
    };
  }
  
  throw new Error(`Resource not found: ${uri}`);
});

// Register tool handlers
server.setRequestHandler(ListToolsRequestSchema, async () => {
  return {
    tools: [
      {
        name: "echo",
        description: "Echo the input text",
        inputSchema: {
          type: "object",
          properties: {
            text: {
              type: "string",
              description: "Text to echo back",
            },
          },
          required: ["text"],
        },
      },
    ],
  };
});

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  console.error(`[TS Server] CallTool called with:`, JSON.stringify(request.params));
  const { name, arguments: args } = request.params;
  
  if (name === "echo") {
    const text = args?.text || "No text provided";
    return {
      content: [
        {
          type: "text",
          text: `Echo: ${text}`,
        },
      ],
    };
  }
  
  throw new Error(`Tool not found: ${name}`);
});

// Register prompt handlers
server.setRequestHandler(ListPromptsRequestSchema, async () => {
  return {
    prompts: [
      {
        name: "greeting",
        description: "Generate a personalized greeting",
        arguments: [
          {
            name: "name",
            description: "The name to greet",
            required: true,
          },
        ],
      },
    ],
  };
});

server.setRequestHandler(GetPromptRequestSchema, async (request) => {
  console.error(`[TS Server] GetPrompt called with:`, JSON.stringify(request.params));
  const { name, arguments: args } = request.params;
  
  if (name === "greeting") {
    const userName = args?.name || "User";
    return {
      description: "A personalized greeting",
      messages: [
        {
          role: "user",
          content: {
            type: "text",
            text: `Hello, ${userName}! Welcome to the TypeScript MCP server.`,
          },
        },
      ],
    };
  }
  
  throw new Error(`Prompt not found: ${name}`);
});

// Start server
async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("[TS Server] TypeScript MCP server started");
}

main().catch((error) => {
  console.error("[TS Server] Fatal error:", error);
  process.exit(1);
});