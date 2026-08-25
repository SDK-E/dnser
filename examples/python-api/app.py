import json, os
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(os.environ.get("PORT", "3002"))

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"service": "python-api", "port": PORT, "path": self.path}).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass

print(f"python-api on :{PORT}")
HTTPServer(("127.0.0.1", PORT), H).serve_forever()
