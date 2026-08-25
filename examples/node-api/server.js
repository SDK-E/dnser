const http = require('http')
const port = Number(process.env.PORT || 3001)
http.createServer((req, res) => {
  res.setHeader('content-type', 'application/json')
  res.end(JSON.stringify({ service: 'node-api', port, path: req.url, ts: Date.now() }))
}).listen(port, () => console.log(`node-api on :${port}`))
