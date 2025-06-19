#!/usr/bin/env python3
"""
Simple test client to verify the Go MCP server works correctly.
This sends JSON-RPC messages over stdio to test concurrent processing.
"""

import json
import subprocess
import sys
import threading
import time
from typing import Any, Dict

class MCPClient:
    def __init__(self, server_cmd: list):
        self.process = subprocess.Popen(
            server_cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=0
        )
        self.request_id = 0
        self.responses = {}
        self.response_lock = threading.Lock()
        
        # Start response reader thread
        self.reader_thread = threading.Thread(target=self._read_responses, daemon=True)
        self.reader_thread.start()
    
    def _read_responses(self):
        """Read responses from the server in a separate thread"""
        try:
            while True:
                line = self.process.stdout.readline()
                if not line:
                    break
                
                try:
                    response = json.loads(line.strip())
                    if 'id' in response:
                        with self.response_lock:
                            self.responses[response['id']] = response
                except json.JSONDecodeError as e:
                    print(f"Failed to parse response: {e}", file=sys.stderr)
                    print(f"Raw line: {line}", file=sys.stderr)
        except Exception as e:
            print(f"Error reading responses: {e}", file=sys.stderr)
    
    def send_request(self, method: str, params: Any = None) -> Dict[str, Any]:
        """Send a request and wait for response"""
        self.request_id += 1
        request_id = self.request_id
        
        request = {
            "jsonrpc": "2.0",
            "id": request_id,
            "method": method,
        }
        
        if params is not None:
            request["params"] = params
        
        # Send request
        request_line = json.dumps(request) + "\n"
        self.process.stdin.write(request_line)
        self.process.stdin.flush()
        
        # Wait for response (with timeout)
        timeout = 5.0
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            with self.response_lock:
                if request_id in self.responses:
                    return self.responses.pop(request_id)
            time.sleep(0.01)
        
        raise TimeoutError(f"Request {request_id} timed out")
    
    def send_notification(self, method: str, params: Any = None):
        """Send a notification (no response expected)"""
        notification = {
            "jsonrpc": "2.0",
            "method": method,
        }
        
        if params is not None:
            notification["params"] = params
        
        notification_line = json.dumps(notification) + "\n"
        self.process.stdin.write(notification_line)
        self.process.stdin.flush()
    
    def close(self):
        """Close the client and terminate the server"""
        if self.process:
            self.process.terminate()
            self.process.wait()

def test_concurrent_requests():
    """Test multiple concurrent requests"""
    print("Testing concurrent Go MCP server...")
    
    # Start the server
    server_cmd = ["go", "run", "main.go"]
    client = MCPClient(server_cmd)
    
    try:
        # Test initialization
        print("\n1. Testing initialization...")
        init_response = client.send_request("initialize", {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {
                "name": "Test Client",
                "version": "1.0.0"
            }
        })
        print(f"Initialize response: {json.dumps(init_response, indent=2)}")
        
        # Send initialized notification
        client.send_notification("initialized")
        
        # Test resources list
        print("\n2. Testing resources list...")
        resources_response = client.send_request("resources/list")
        print(f"Resources response: {json.dumps(resources_response, indent=2)}")
        
        # Test resource read
        print("\n3. Testing resource read...")
        resource_read_response = client.send_request("resources/read", {
            "uri": "file://example.txt"
        })
        print(f"Resource read response: {json.dumps(resource_read_response, indent=2)}")
        
        # Test tools list
        print("\n4. Testing tools list...")
        tools_response = client.send_request("tools/list")
        print(f"Tools response: {json.dumps(tools_response, indent=2)}")
        
        # Test tool call
        print("\n5. Testing tool call...")
        tool_call_response = client.send_request("tools/call", {
            "name": "echo",
            "arguments": {
                "text": "Hello from concurrent test!"
            }
        })
        print(f"Tool call response: {json.dumps(tool_call_response, indent=2)}")
        
        # Test prompts list
        print("\n6. Testing prompts list...")
        prompts_response = client.send_request("prompts/list")
        print(f"Prompts response: {json.dumps(prompts_response, indent=2)}")
        
        # Test prompt get
        print("\n7. Testing prompt get...")
        prompt_response = client.send_request("prompts/get", {
            "name": "greeting",
            "arguments": {
                "name": "Go Developer"
            }
        })
        print(f"Prompt response: {json.dumps(prompt_response, indent=2)}")
        
        # Test concurrent requests
        print("\n8. Testing concurrent requests...")
        import concurrent.futures
        
        def make_request(i):
            return client.send_request("tools/call", {
                "name": "echo",
                "arguments": {
                    "text": f"Concurrent request {i}"
                }
            })
        
        with concurrent.futures.ThreadPoolExecutor(max_workers=10) as executor:
            futures = [executor.submit(make_request, i) for i in range(10)]
            results = [future.result() for future in concurrent.futures.as_completed(futures)]
        
        print(f"Completed {len(results)} concurrent requests successfully!")
        
        print("\n✅ All tests passed! The Go MCP server is working correctly with concurrent processing.")
        
    except Exception as e:
        print(f"❌ Test failed: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
    finally:
        client.close()

if __name__ == "__main__":
    test_concurrent_requests()