# MCP Go SDK Examples

Comprehensive examples showing how to build MCP servers and clients with the Go SDK.

## Quick Start Examples

### Simple Echo Server

A basic MCP server that echoes back tool arguments:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
)

func main() {
    ctx := context.Background()

    // Create stdio transport
    stdioTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{})
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create transport: %v\n", err)
        os.Exit(1)
    }

    // Create server
    srv := server.NewServer(ctx, stdioTransport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "echo-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools: &shared.ToolsCapability{},
        },
    })

    // Register echo tool
    srv.RegisterTool(shared.Tool{
        Name:        "echo",
        Description: "Echo back the input",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "message": map[string]interface{}{
                    "type":        "string",
                    "description": "Message to echo back",
                },
            },
            "required": []string{"message"},
        },
    })

    srv.SetToolHandler("echo", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        message := arguments["message"].(string)
        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: fmt.Sprintf("Echo: %s", message),
            },
        }, nil
    })

    // Start server
    if err := srv.Serve(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
        os.Exit(1)
    }
}
```

### Simple Client

A basic client that connects to an MCP server:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/rubys/mcp-go-sdk/client"
    "github.com/rubys/mcp-go-sdk/shared"
)

func main() {
    ctx := context.Background()

    // Create process transport to spawn echo server
    processTransport, err := client.NewProcessTransport(ctx, client.ProcessConfig{
        Command: "go",
        Args:    []string{"run", "echo-server.go"},
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create transport: %v\n", err)
        os.Exit(1)
    }

    // Create client
    mcpClient := client.New(processTransport)
    mcpClient.config = client.ClientConfig{
        ClientInfo: shared.ClientInfo{
            Name:    "echo-client",
            Version: "1.0.0",
        },
        Capabilities: shared.ClientCapabilities{},
    }

    // Start client
    if err := mcpClient.Start(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to start client: %v\n", err)
        os.Exit(1)
    }
    defer mcpClient.Close()

    // List available tools
    tools, err := mcpClient.ListTools(ctx)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("Available tools: %d\n", len(tools.Tools))
    for _, tool := range tools.Tools {
        fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
    }

    // Call echo tool
    result, err := mcpClient.CallTool(ctx, shared.CallToolRequest{
        Name: "echo",
        Arguments: map[string]interface{}{
            "message": "Hello, MCP!",
        },
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to call tool: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("Tool result: %s\n", result.Content[0].(shared.TextContent).Text)
}
```

## Advanced Examples

### File System Server

A server that provides file system access through MCP:

```go
package main

import (
    "context"
    "fmt"
    "io/fs"
    "os"
    "path/filepath"
    "strings"

    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
)

type FileSystemServer struct {
    rootPath string
    server   *server.Server
}

func NewFileSystemServer(rootPath string) *FileSystemServer {
    ctx := context.Background()

    stdioTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{})
    if err != nil {
        panic(err)
    }

    srv := server.NewServer(ctx, stdioTransport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "filesystem-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools:     &shared.ToolsCapability{},
            Resources: &shared.ResourcesCapability{},
        },
    })

    fs := &FileSystemServer{
        rootPath: rootPath,
        server:   srv,
    }

    fs.setupTools()
    fs.setupResources()

    return fs
}

func (fs *FileSystemServer) setupTools() {
    // List files tool
    fs.server.RegisterTool(shared.Tool{
        Name:        "list_files",
        Description: "List files in a directory",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "path": map[string]interface{}{
                    "type":        "string",
                    "description": "Directory path to list",
                    "default":     ".",
                },
            },
        },
    })

    fs.server.SetToolHandler("list_files", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        dirPath := "."
        if path, ok := arguments["path"].(string); ok {
            dirPath = path
        }

        fullPath := filepath.Join(fs.rootPath, dirPath)
        
        entries, err := os.ReadDir(fullPath)
        if err != nil {
            return nil, fmt.Errorf("failed to read directory: %w", err)
        }

        var files []string
        for _, entry := range entries {
            info, err := entry.Info()
            if err != nil {
                continue
            }

            prefix := ""
            if entry.IsDir() {
                prefix = "[DIR] "
            }

            files = append(files, fmt.Sprintf("%s%s (%d bytes)", prefix, entry.Name(), info.Size()))
        }

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: fmt.Sprintf("Files in %s:\n%s", dirPath, strings.Join(files, "\n")),
            },
        }, nil
    })

    // Read file tool
    fs.server.RegisterTool(shared.Tool{
        Name:        "read_file",
        Description: "Read contents of a file",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "path": map[string]interface{}{
                    "type":        "string",
                    "description": "File path to read",
                },
            },
            "required": []string{"path"},
        },
    })

    fs.server.SetToolHandler("read_file", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        filePath := arguments["path"].(string)
        fullPath := filepath.Join(fs.rootPath, filePath)

        content, err := os.ReadFile(fullPath)
        if err != nil {
            return nil, fmt.Errorf("failed to read file: %w", err)
        }

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: string(content),
                URI:  fmt.Sprintf("file://%s", fullPath),
            },
        }, nil
    })

    // Write file tool
    fs.server.RegisterTool(shared.Tool{
        Name:        "write_file",
        Description: "Write content to a file",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "path": map[string]interface{}{
                    "type":        "string",
                    "description": "File path to write",
                },
                "content": map[string]interface{}{
                    "type":        "string",
                    "description": "Content to write",
                },
            },
            "required": []string{"path", "content"},
        },
    })

    fs.server.SetToolHandler("write_file", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        filePath := arguments["path"].(string)
        content := arguments["content"].(string)
        fullPath := filepath.Join(fs.rootPath, filePath)

        // Ensure directory exists
        if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
            return nil, fmt.Errorf("failed to create directory: %w", err)
        }

        if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
            return nil, fmt.Errorf("failed to write file: %w", err)
        }

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), filePath),
            },
        }, nil
    })
}

func (fs *FileSystemServer) setupResources() {
    // Auto-discover files as resources
    fs.server.SetResourceHandler("file://", func(ctx context.Context, uri string) ([]shared.Content, error) {
        filePath := strings.TrimPrefix(uri, "file://")
        fullPath := filepath.Join(fs.rootPath, filePath)

        content, err := os.ReadFile(fullPath)
        if err != nil {
            return nil, fmt.Errorf("failed to read file: %w", err)
        }

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: string(content),
                URI:  uri,
            },
        }, nil
    })

    // Dynamically list files as resources
    fs.server.SetListResourcesHandler(func(ctx context.Context) ([]shared.Resource, error) {
        var resources []shared.Resource

        err := filepath.WalkDir(fs.rootPath, func(path string, d fs.DirEntry, err error) error {
            if err != nil {
                return err
            }

            if !d.IsDir() {
                relPath, err := filepath.Rel(fs.rootPath, path)
                if err != nil {
                    return err
                }

                resources = append(resources, shared.Resource{
                    URI:         fmt.Sprintf("file://%s", relPath),
                    Name:        filepath.Base(relPath),
                    Description: fmt.Sprintf("File: %s", relPath),
                    MimeType:    "text/plain",
                })
            }
            return nil
        })

        if err != nil {
            return nil, fmt.Errorf("failed to walk directory: %w", err)
        }

        return resources, nil
    })
}

func (fs *FileSystemServer) Serve(ctx context.Context) error {
    return fs.server.Serve(ctx)
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintf(os.Stderr, "Usage: %s <root-directory>\n", os.Args[0])
        os.Exit(1)
    }

    rootPath := os.Args[1]
    if _, err := os.Stat(rootPath); os.IsNotExist(err) {
        fmt.Fprintf(os.Stderr, "Directory does not exist: %s\n", rootPath)
        os.Exit(1)
    }

    fs := NewFileSystemServer(rootPath)
    if err := fs.Serve(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
        os.Exit(1)
    }
}
```

### Database Server

A server that provides database access through MCP:

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "os"
    "strings"

    _ "github.com/mattn/go-sqlite3"
    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
)

type DatabaseServer struct {
    db     *sql.DB
    server *server.Server
}

func NewDatabaseServer(dbPath string) (*DatabaseServer, error) {
    ctx := context.Background()

    // Open database
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }

    // Create transport
    stdioTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{})
    if err != nil {
        return nil, err
    }

    // Create server
    srv := server.NewServer(ctx, stdioTransport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "database-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools: &shared.ToolsCapability{},
        },
    })

    ds := &DatabaseServer{
        db:     db,
        server: srv,
    }

    ds.setupTools()
    return ds, nil
}

func (ds *DatabaseServer) setupTools() {
    // Query tool
    ds.server.RegisterTool(shared.Tool{
        Name:        "query",
        Description: "Execute a SQL query",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "sql": map[string]interface{}{
                    "type":        "string",
                    "description": "SQL query to execute",
                },
                "params": map[string]interface{}{
                    "type":        "array",
                    "description": "Query parameters",
                    "items": map[string]interface{}{
                        "type": "string",
                    },
                },
            },
            "required": []string{"sql"},
        },
    })

    ds.server.SetToolHandler("query", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        sqlQuery := arguments["sql"].(string)
        
        var params []interface{}
        if paramsInterface, ok := arguments["params"].([]interface{}); ok {
            params = paramsInterface
        }

        // Execute query
        rows, err := ds.db.QueryContext(ctx, sqlQuery, params...)
        if err != nil {
            return nil, fmt.Errorf("query failed: %w", err)
        }
        defer rows.Close()

        // Get column names
        columns, err := rows.Columns()
        if err != nil {
            return nil, fmt.Errorf("failed to get columns: %w", err)
        }

        // Prepare result
        var results []map[string]interface{}
        
        for rows.Next() {
            // Create slice for scanning
            values := make([]interface{}, len(columns))
            valuePtrs := make([]interface{}, len(columns))
            for i := range values {
                valuePtrs[i] = &values[i]
            }

            // Scan row
            if err := rows.Scan(valuePtrs...); err != nil {
                return nil, fmt.Errorf("failed to scan row: %w", err)
            }

            // Build result map
            row := make(map[string]interface{})
            for i, col := range columns {
                val := values[i]
                if b, ok := val.([]byte); ok {
                    val = string(b)
                }
                row[col] = val
            }
            results = append(results, row)
        }

        if err := rows.Err(); err != nil {
            return nil, fmt.Errorf("rows iteration error: %w", err)
        }

        // Format output
        var output strings.Builder
        output.WriteString(fmt.Sprintf("Query executed successfully. %d rows returned.\n\n", len(results)))
        
        if len(results) > 0 {
            // Write headers
            output.WriteString(strings.Join(columns, " | "))
            output.WriteString("\n")
            output.WriteString(strings.Repeat("-", len(strings.Join(columns, " | "))))
            output.WriteString("\n")

            // Write data
            for _, row := range results {
                var values []string
                for _, col := range columns {
                    values = append(values, fmt.Sprintf("%v", row[col]))
                }
                output.WriteString(strings.Join(values, " | "))
                output.WriteString("\n")
            }
        }

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: output.String(),
            },
        }, nil
    })

    // Execute tool (for non-SELECT queries)
    ds.server.RegisterTool(shared.Tool{
        Name:        "execute",
        Description: "Execute a SQL statement (INSERT, UPDATE, DELETE, etc.)",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "sql": map[string]interface{}{
                    "type":        "string",
                    "description": "SQL statement to execute",
                },
                "params": map[string]interface{}{
                    "type":        "array",
                    "description": "Statement parameters",
                    "items": map[string]interface{}{
                        "type": "string",
                    },
                },
            },
            "required": []string{"sql"},
        },
    })

    ds.server.SetToolHandler("execute", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        sqlStatement := arguments["sql"].(string)
        
        var params []interface{}
        if paramsInterface, ok := arguments["params"].([]interface{}); ok {
            params = paramsInterface
        }

        // Execute statement
        result, err := ds.db.ExecContext(ctx, sqlStatement, params...)
        if err != nil {
            return nil, fmt.Errorf("execution failed: %w", err)
        }

        rowsAffected, _ := result.RowsAffected()
        lastInsertId, _ := result.LastInsertId()

        output := fmt.Sprintf("Statement executed successfully.\nRows affected: %d", rowsAffected)
        if lastInsertId > 0 {
            output += fmt.Sprintf("\nLast insert ID: %d", lastInsertId)
        }

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: output,
            },
        }, nil
    })

    // List tables tool
    ds.server.RegisterTool(shared.Tool{
        Name:        "list_tables",
        Description: "List all tables in the database",
        InputSchema: map[string]interface{}{
            "type": "object",
        },
    })

    ds.server.SetToolHandler("list_tables", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        rows, err := ds.db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
        if err != nil {
            return nil, fmt.Errorf("failed to list tables: %w", err)
        }
        defer rows.Close()

        var tables []string
        for rows.Next() {
            var tableName string
            if err := rows.Scan(&tableName); err != nil {
                return nil, fmt.Errorf("failed to scan table name: %w", err)
            }
            tables = append(tables, tableName)
        }

        output := fmt.Sprintf("Tables in database (%d):\n%s", len(tables), strings.Join(tables, "\n"))

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: output,
            },
        }, nil
    })
}

func (ds *DatabaseServer) Serve(ctx context.Context) error {
    defer ds.db.Close()
    return ds.server.Serve(ctx)
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintf(os.Stderr, "Usage: %s <database-file>\n", os.Args[0])
        os.Exit(1)
    }

    dbPath := os.Args[1]
    
    ds, err := NewDatabaseServer(dbPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create database server: %v\n", err)
        os.Exit(1)
    }

    if err := ds.Serve(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
        os.Exit(1)
    }
}
```

### HTTP API Server

A server that provides HTTP API access through MCP:

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strconv"
    "time"

    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
)

type HTTPAPIServer struct {
    httpClient *http.Client
    server     *server.Server
    baseURL    string
}

func NewHTTPAPIServer(baseURL string) *HTTPAPIServer {
    ctx := context.Background()

    stdioTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{})
    if err != nil {
        panic(err)
    }

    srv := server.NewServer(ctx, stdioTransport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "http-api-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools: &shared.ToolsCapability{},
        },
    })

    httpServer := &HTTPAPIServer{
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
        server:  srv,
        baseURL: baseURL,
    }

    httpServer.setupTools()
    return httpServer
}

func (h *HTTPAPIServer) setupTools() {
    // GET request tool
    h.server.RegisterTool(shared.Tool{
        Name:        "http_get",
        Description: "Make an HTTP GET request",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "path": map[string]interface{}{
                    "type":        "string",
                    "description": "API endpoint path",
                },
                "headers": map[string]interface{}{
                    "type":        "object",
                    "description": "Request headers",
                },
            },
            "required": []string{"path"},
        },
    })

    h.server.SetToolHandler("http_get", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        path := arguments["path"].(string)
        url := h.baseURL + path

        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            return nil, fmt.Errorf("failed to create request: %w", err)
        }

        // Add headers if provided
        if headers, ok := arguments["headers"].(map[string]interface{}); ok {
            for key, value := range headers {
                req.Header.Set(key, fmt.Sprintf("%v", value))
            }
        }

        resp, err := h.httpClient.Do(req)
        if err != nil {
            return nil, fmt.Errorf("request failed: %w", err)
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        if err != nil {
            return nil, fmt.Errorf("failed to read response: %w", err)
        }

        // Format response
        output := fmt.Sprintf("Status: %s\nHeaders:\n", resp.Status)
        for key, values := range resp.Header {
            output += fmt.Sprintf("  %s: %s\n", key, values[0])
        }
        output += fmt.Sprintf("\nBody:\n%s", string(body))

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: output,
            },
        }, nil
    })

    // POST request tool
    h.server.RegisterTool(shared.Tool{
        Name:        "http_post",
        Description: "Make an HTTP POST request",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "path": map[string]interface{}{
                    "type":        "string",
                    "description": "API endpoint path",
                },
                "body": map[string]interface{}{
                    "type":        "object",
                    "description": "Request body (JSON)",
                },
                "headers": map[string]interface{}{
                    "type":        "object",
                    "description": "Request headers",
                },
            },
            "required": []string{"path"},
        },
    })

    h.server.SetToolHandler("http_post", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        path := arguments["path"].(string)
        url := h.baseURL + path

        var bodyReader io.Reader
        if bodyData, ok := arguments["body"]; ok {
            jsonData, err := json.Marshal(bodyData)
            if err != nil {
                return nil, fmt.Errorf("failed to marshal body: %w", err)
            }
            bodyReader = bytes.NewReader(jsonData)
        }

        req, err := http.NewRequestWithContext(ctx, "POST", url, bodyReader)
        if err != nil {
            return nil, fmt.Errorf("failed to create request: %w", err)
        }

        // Set default content type
        req.Header.Set("Content-Type", "application/json")

        // Add headers if provided
        if headers, ok := arguments["headers"].(map[string]interface{}); ok {
            for key, value := range headers {
                req.Header.Set(key, fmt.Sprintf("%v", value))
            }
        }

        resp, err := h.httpClient.Do(req)
        if err != nil {
            return nil, fmt.Errorf("request failed: %w", err)
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        if err != nil {
            return nil, fmt.Errorf("failed to read response: %w", err)
        }

        // Format response
        output := fmt.Sprintf("Status: %s\nHeaders:\n", resp.Status)
        for key, values := range resp.Header {
            output += fmt.Sprintf("  %s: %s\n", key, values[0])
        }
        output += fmt.Sprintf("\nBody:\n%s", string(body))

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: output,
            },
        }, nil
    })
}

func (h *HTTPAPIServer) Serve(ctx context.Context) error {
    return h.server.Serve(ctx)
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintf(os.Stderr, "Usage: %s <base-url>\n", os.Args[0])
        fmt.Fprintf(os.Stderr, "Example: %s https://api.example.com\n", os.Args[0])
        os.Exit(1)
    }

    baseURL := os.Args[1]
    
    server := NewHTTPAPIServer(baseURL)
    if err := server.Serve(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
        os.Exit(1)
    }
}
```

### WebSocket Server

A real-time server using WebSocket transport:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
)

type WebSocketServer struct {
    upgrader websocket.Upgrader
    clients  map[*websocket.Conn]*server.Server
    mu       sync.RWMutex
}

func NewWebSocketServer() *WebSocketServer {
    return &WebSocketServer{
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool {
                return true // Allow all origins in this example
            },
        },
        clients: make(map[*websocket.Conn]*server.Server),
    }
}

func (ws *WebSocketServer) HandleConnection(w http.ResponseWriter, r *http.Request) {
    conn, err := ws.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("WebSocket upgrade failed: %v", err)
        return
    }
    defer conn.Close()

    ctx := context.Background()

    // Create WebSocket transport
    wsTransport, err := transport.NewWebSocketTransport(transport.WebSocketConfig{
        Conn: conn,
    })
    if err != nil {
        log.Printf("Failed to create WebSocket transport: %v", err)
        return
    }

    // Create MCP server for this connection
    srv := server.NewServer(ctx, wsTransport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "websocket-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools:     &shared.ToolsCapability{},
            Resources: &shared.ResourcesCapability{},
        },
    })

    // Register tools
    ws.setupTools(srv)

    // Track client
    ws.mu.Lock()
    ws.clients[conn] = srv
    ws.mu.Unlock()

    defer func() {
        ws.mu.Lock()
        delete(ws.clients, conn)
        ws.mu.Unlock()
    }()

    log.Printf("New WebSocket connection established")

    // Serve the connection
    if err := srv.Serve(ctx); err != nil {
        log.Printf("Server error for WebSocket connection: %v", err)
    }

    log.Printf("WebSocket connection closed")
}

func (ws *WebSocketServer) setupTools(srv *server.Server) {
    // Get current time tool
    srv.RegisterTool(shared.Tool{
        Name:        "get_time",
        Description: "Get the current server time",
        InputSchema: map[string]interface{}{
            "type": "object",
        },
    })

    srv.SetToolHandler("get_time", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        now := time.Now()
        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: fmt.Sprintf("Current time: %s", now.Format(time.RFC3339)),
            },
        }, nil
    })

    // Broadcast message tool
    srv.RegisterTool(shared.Tool{
        Name:        "broadcast",
        Description: "Broadcast a message to all connected clients",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "message": map[string]interface{}{
                    "type":        "string",
                    "description": "Message to broadcast",
                },
            },
            "required": []string{"message"},
        },
    })

    srv.SetToolHandler("broadcast", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        message := arguments["message"].(string)
        
        ws.mu.RLock()
        clientCount := len(ws.clients)
        ws.mu.RUnlock()

        // In a real implementation, you would send notifications to all clients
        // This is simplified for the example
        
        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: fmt.Sprintf("Broadcast message '%s' sent to %d clients", message, clientCount),
            },
        }, nil
    })

    // Get client count tool
    srv.RegisterTool(shared.Tool{
        Name:        "client_count",
        Description: "Get the number of connected clients",
        InputSchema: map[string]interface{}{
            "type": "object",
        },
    })

    srv.SetToolHandler("client_count", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        ws.mu.RLock()
        count := len(ws.clients)
        ws.mu.RUnlock()

        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: fmt.Sprintf("Connected clients: %d", count),
            },
        }, nil
    })
}

func main() {
    port := "8080"
    if len(os.Args) > 1 {
        port = os.Args[1]
    }

    wsServer := NewWebSocketServer()

    http.HandleFunc("/mcp", wsServer.HandleConnection)
    
    // Serve static files for testing
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        html := `
<!DOCTYPE html>
<html>
<head>
    <title>MCP WebSocket Test</title>
</head>
<body>
    <h1>MCP WebSocket Test</h1>
    <div id="status">Connecting...</div>
    <div id="messages"></div>
    <script>
        const ws = new WebSocket('ws://localhost:` + port + `/mcp');
        const status = document.getElementById('status');
        const messages = document.getElementById('messages');
        
        ws.onopen = function() {
            status.textContent = 'Connected';
            // Send initialize request
            ws.send(JSON.stringify({
                jsonrpc: '2.0',
                id: 1,
                method: 'initialize',
                params: {
                    protocolVersion: '2024-11-05',
                    capabilities: {},
                    clientInfo: { name: 'web-client', version: '1.0.0' }
                }
            }));
        };
        
        ws.onmessage = function(event) {
            const div = document.createElement('div');
            div.textContent = event.data;
            messages.appendChild(div);
        };
        
        ws.onclose = function() {
            status.textContent = 'Disconnected';
        };
    </script>
</body>
</html>`
        w.Header().Set("Content-Type", "text/html")
        w.Write([]byte(html))
    })

    addr := ":" + port
    log.Printf("Starting WebSocket server on %s", addr)
    log.Printf("Test page available at http://localhost%s", addr)
    
    if err := http.ListenAndServe(addr, nil); err != nil {
        log.Fatalf("Server failed: %v", err)
    }
}
```

## Testing Examples

### Unit Testing

```go
package main

import (
    "context"
    "testing"

    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestEchoServer(t *testing.T) {
    ctx := context.Background()

    // Create in-process transport for testing
    inProcessTransport := transport.NewInProcessTransport()

    // Create and configure server
    srv := server.NewServer(ctx, inProcessTransport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "test-echo-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools: &shared.ToolsCapability{},
        },
    })

    // Register echo tool
    srv.RegisterTool(shared.Tool{
        Name:        "echo",
        Description: "Echo back the input",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "message": map[string]interface{}{
                    "type": "string",
                },
            },
            "required": []string{"message"},
        },
    })

    srv.SetToolHandler("echo", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        message := arguments["message"].(string)
        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: "Echo: " + message,
            },
        }, nil
    })

    // Start server in background
    go func() {
        srv.Serve(ctx)
    }()

    // Test tool call
    result, err := inProcessTransport.CallTool(ctx, "echo", map[string]interface{}{
        "message": "Hello, World!",
    })
    require.NoError(t, err)
    assert.Len(t, result, 1)
    assert.Equal(t, "Echo: Hello, World!", result[0].(shared.TextContent).Text)
}
```

## Performance Examples

### Concurrent Tool Handler

```go
func (s *MyServer) setupConcurrentTool() {
    s.server.RegisterTool(shared.Tool{
        Name:        "parallel_process",
        Description: "Process multiple items in parallel",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "items": map[string]interface{}{
                    "type": "array",
                    "items": map[string]interface{}{
                        "type": "string",
                    },
                },
            },
            "required": []string{"items"},
        },
    })

    s.server.SetToolHandler("parallel_process", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        items := arguments["items"].([]interface{})
        
        // Process items concurrently
        type result struct {
            index int
            value string
            err   error
        }
        
        resultChan := make(chan result, len(items))
        semaphore := make(chan struct{}, 10) // Limit concurrent goroutines
        
        for i, item := range items {
            go func(index int, data interface{}) {
                semaphore <- struct{}{} // Acquire
                defer func() { <-semaphore }() // Release
                
                // Simulate processing
                processed := fmt.Sprintf("Processed: %v", data)
                resultChan <- result{index: index, value: processed}
            }(i, item)
        }
        
        // Collect results
        results := make([]string, len(items))
        for i := 0; i < len(items); i++ {
            r := <-resultChan
            if r.err != nil {
                return nil, r.err
            }
            results[r.index] = r.value
        }
        
        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: strings.Join(results, "\n"),
            },
        }, nil
    })
}
```

These examples demonstrate the power and flexibility of the Go MCP SDK. They show how to build high-performance, concurrent MCP servers that can handle real-world use cases while maintaining the simplicity and reliability that Go is known for.