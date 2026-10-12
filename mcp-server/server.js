const readline = require('node:readline');
const fs = require('node:fs');
const path = require('node:path');

// The lectures, read once when the server starts.
const lectures = JSON.parse(fs.readFileSync(path.join(__dirname, 'lectures.json'), 'utf8'));

// A tool result: one text part, and whether the tool failed.
function textResult(id, text, isError) {
  return { jsonrpc: '2.0', id: id, result: { content: [{ type: 'text', text: text }], isError: isError } };
}

// The tools this server offers. The description is what the model reads to choose.
const tools = [
  {
    name: 'find_lecture',
    description: 'Return the title and topics of one course lecture by its number, 1 to 10. Use it when asked what a lecture covers.',
    inputSchema: {
      type: 'object',
      properties: { number: { type: 'integer', description: 'Lecture number, 1 to 10' } },
      required: ['number'],
    },
  },
  {
    name: 'list_lectures',
    description: 'List the number and title of every course lecture. Use it when asked what the course covers.',
    inputSchema: { type: 'object', properties: {} },
  },
];

// Print one reply as one line of JSON on stdout.
function send(reply) {
  process.stdout.write(JSON.stringify(reply) + '\n');
}

// Decide what to answer. Return the reply, or null to answer nothing.
function handle(msg) {
  if (msg.method === 'initialize') {
    return {
      jsonrpc: '2.0',
      id: msg.id,
      result: {
        protocolVersion: msg.params.protocolVersion,
        capabilities: { tools: {} },
        serverInfo: { name: 'lectures', version: '0.1.0' },
      },
    };
  }
  if (msg.method === 'notifications/initialized') {
    return null; // a notification: it has no id and gets no reply
  }
  if (msg.method === 'tools/list') {
    return { jsonrpc: '2.0', id: msg.id, result: { tools: tools } };
  }
  if (msg.method === 'tools/call') {
    const name = msg.params.name;
    const args = msg.params.arguments || {};
    if (name === 'find_lecture') {
      const lecture = lectures.find((l) => l.number === args.number);
      if (!lecture) {
        // The tool ran and failed: a normal result the model can read, marked isError.
        return textResult(msg.id, 'There is no lecture ' + args.number + '. The course has lectures 1 to 10.', true);
      }
      return textResult(msg.id, 'Lecture ' + lecture.number + ': ' + lecture.title + '. Topics: ' + lecture.topics, false);
    }
    if (name === 'list_lectures') {
      return textResult(msg.id, lectures.map((l) => l.number + '. ' + l.title).join('\n'), false);
    }
    // A tool we never offered: the message itself is wrong, so it is a protocol error.
    return { jsonrpc: '2.0', id: msg.id, error: { code: -32602, message: 'Unknown tool: ' + name } };
  }
  if (msg.id !== undefined) {
    // Any other request we do not know.
    return { jsonrpc: '2.0', id: msg.id, error: { code: -32601, message: 'Unknown method: ' + msg.method } };
  }
  return null;
}

const lines = readline.createInterface({ input: process.stdin });

lines.on('line', (line) => {
  console.error('got: ' + line);
  const reply = handle(JSON.parse(line));
  if (reply) send(reply);
});