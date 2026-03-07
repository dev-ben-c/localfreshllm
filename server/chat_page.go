package server

import "net/http"

const chatPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>LocalFresh Chat</title>
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #1a1a2e;
  --surface: #16213e;
  --border: #0f3460;
  --text: #e0e0e0;
  --muted: #888;
  --accent: #e0a526;
  --user-bg: #0f3460;
  --assistant-bg: #1a1a2e;
  --tool-bg: #0d1b2a;
  --error: #e74c3c;
  --success: #2ecc71;
}
html, body { height: 100%; background: var(--bg); color: var(--text); font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
body { display: flex; flex-direction: column; }

/* Toolbar */
#toolbar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: var(--surface); border-bottom: 1px solid var(--border); flex-wrap: wrap; }
#toolbar select, #toolbar input[type="text"] {
  background: var(--bg); color: var(--text); border: 1px solid var(--border); border-radius: 6px;
  padding: 5px 8px; font-size: 12px; outline: none;
}
#toolbar select:focus, #toolbar input:focus { border-color: var(--accent); }
#toolbar label { font-size: 12px; color: var(--muted); white-space: nowrap; }
.toolbar-group { display: flex; align-items: center; gap: 4px; }
.toolbar-sep { width: 1px; height: 20px; background: var(--border); margin: 0 4px; }
#model-select { max-width: 180px; }
#location-input { width: 120px; }
.toggle-btn {
  background: var(--bg); color: var(--muted); border: 1px solid var(--border); border-radius: 6px;
  padding: 4px 10px; font-size: 12px; cursor: pointer; transition: all 0.15s;
}
.toggle-btn.active { background: var(--accent); color: #000; border-color: var(--accent); }
.toggle-btn:hover { border-color: var(--accent); }
#location-save { background: var(--accent); color: #000; border: none; border-radius: 6px; padding: 4px 8px; font-size: 12px; cursor: pointer; }
#location-save:hover { opacity: 0.9; }
.saved-indicator { color: var(--success); font-size: 11px; opacity: 0; transition: opacity 0.3s; }
.saved-indicator.show { opacity: 1; }
#voice-status { font-size: 11px; color: var(--muted); margin-left: 4px; }
#voice-status.listening { color: var(--success); }
#voice-status.processing { color: var(--accent); }

/* Chat */
#chat-container { flex: 1; overflow-y: auto; padding: 12px; display: flex; flex-direction: column; gap: 8px; }
.msg { max-width: 85%; padding: 10px 14px; border-radius: 12px; line-height: 1.5; font-size: 14px; word-wrap: break-word; white-space: pre-wrap; }
.msg.user { align-self: flex-end; background: var(--user-bg); border: 1px solid var(--border); }
.msg.assistant { align-self: flex-start; background: var(--assistant-bg); border: 1px solid var(--border); }
.msg.tool { align-self: flex-start; background: var(--tool-bg); border: 1px solid var(--border); font-size: 12px; font-family: monospace; opacity: 0.8; max-height: 120px; overflow-y: auto; }
.msg.error { align-self: center; color: var(--error); font-size: 12px; }
.msg.system { align-self: center; color: var(--muted); font-size: 12px; font-style: italic; }
.tool-label { color: var(--accent); font-weight: bold; font-size: 11px; text-transform: uppercase; margin-bottom: 4px; }

/* Input area */
#input-area { display: flex; align-items: flex-end; gap: 8px; padding: 12px; background: var(--surface); border-top: 1px solid var(--border); }
#message-input { flex: 1; background: var(--bg); color: var(--text); border: 1px solid var(--border); border-radius: 8px; padding: 10px 14px; font-size: 14px; resize: none; outline: none; min-height: 42px; max-height: 120px; font-family: inherit; }
#message-input:focus { border-color: var(--accent); }
#send-btn { background: var(--accent); color: #000; border: none; border-radius: 8px; padding: 10px 16px; font-size: 14px; font-weight: 600; cursor: pointer; white-space: nowrap; }
#send-btn:hover { opacity: 0.9; }
#send-btn:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
</head>
<body>

<div id="toolbar">
  <div class="toolbar-group">
    <label for="model-select">Model</label>
    <select id="model-select"><option>loading...</option></select>
  </div>
  <div class="toolbar-sep"></div>
  <div class="toolbar-group">
    <label for="location-input">Location</label>
    <input type="text" id="location-input" placeholder="City or zip">
    <button id="location-save">Set</button>
    <span id="location-saved" class="saved-indicator">saved</span>
  </div>
  <div class="toolbar-sep"></div>
  <div class="toolbar-group">
    <button id="tts-toggle" class="toggle-btn" title="Read responses aloud">TTS</button>
    <button id="voice-toggle" class="toggle-btn" title="Always-on voice (wake word: Cedric)">Voice</button>
    <span id="voice-status"></span>
  </div>
</div>

<div id="chat-container"></div>

<div id="input-area">
  <textarea id="message-input" placeholder="Ask me anything..." rows="1"></textarea>
  <button id="send-btn">Send</button>
</div>

<script>
const params = new URLSearchParams(window.location.search);
const TOKEN = params.get('token') || '';
const BASE = window.location.origin;
const authHeaders = { 'Authorization': 'Bearer ' + TOKEN };

const container = document.getElementById('chat-container');
const input = document.getElementById('message-input');
const sendBtn = document.getElementById('send-btn');
const modelSelect = document.getElementById('model-select');
const locationInput = document.getElementById('location-input');
const locationSave = document.getElementById('location-save');
const locationSaved = document.getElementById('location-saved');
const ttsToggle = document.getElementById('tts-toggle');
const voiceToggle = document.getElementById('voice-toggle');
const voiceStatus = document.getElementById('voice-status');

let sessionId = '';
let sending = false;
let ttsEnabled = false;
let voiceEnabled = false;
let audioStream = null;
let audioContext = null;
let analyser = null;
let voiceProcessor = null;
let speechBuffer = [];
let silenceCount = 0;
let isSpeaking = false;
const RMS_THRESHOLD = 0.015;
const SILENCE_CHUNKS = 15;
const MIN_SPEECH_MS = 500;
const CHUNK_MS = 100;
const SAMPLE_RATE = 16000;
const LEAD_IN_CHUNKS = 3;
let leadInRing = [];

// --- Init ---

async function init() {
  await Promise.all([loadModels(), loadDevice()]);
}

async function loadModels() {
  try {
    const resp = await fetch(BASE + '/v1/models', { headers: authHeaders, credentials: 'same-origin' });
    if (!resp.ok) {
      console.error('Model load failed:', resp.status, resp.statusText);
      modelSelect.innerHTML = '<option>(auth error - check token)</option>';
      return;
    }
    const data = await resp.json();
    const models = data.models || [];
    modelSelect.innerHTML = '';
    if (models.length === 0) {
      modelSelect.innerHTML = '<option>(no models)</option>';
      return;
    }
    for (const m of models) {
      const opt = document.createElement('option');
      opt.value = m; opt.textContent = m;
      modelSelect.appendChild(opt);
    }
  } catch (err) {
    console.error('Model load error:', err);
    modelSelect.innerHTML = '<option>(load failed)</option>';
  }
}

async function loadDevice() {
  try {
    const resp = await fetch(BASE + '/v1/devices/me', { headers: authHeaders, credentials: 'same-origin' });
    if (!resp.ok) return;
    const dev = await resp.json();
    if (dev.location) locationInput.value = dev.location;
    if (dev.model) modelSelect.value = dev.model;
  } catch {}
}

// --- Location ---

async function saveLocation() {
  const loc = locationInput.value.trim();
  try {
    await fetch(BASE + '/v1/devices/me', {
      method: 'PUT',
      headers: { ...authHeaders, 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ location: loc })
    });
    locationSaved.classList.add('show');
    setTimeout(() => locationSaved.classList.remove('show'), 2000);
  } catch {}
}

locationSave.addEventListener('click', saveLocation);
locationInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') { e.preventDefault(); saveLocation(); }
});

// --- TTS Sanitization (mirrors tui/audio.go) ---

function sanitizeForTTS(text) {
  // Remove code blocks
  text = text.replace(/` + "```" + `[\\s\\S]*?` + "```" + `/g, ' ');
  text = text.replace(/` + "`" + `[^` + "`" + `]+` + "`" + `/g, ' ');
  // Remove URLs
  text = text.replace(/https?:\/\/\\S+/g, ' ');
  // Remove markdown formatting
  text = text.replace(/^#{1,6}\\s+/gm, '');
  text = text.replace(/\\*{1,3}/g, '');
  text = text.replace(/^\\s*[-*+]\\s+/gm, '');
  text = text.replace(/^\\s*\\d+\\.\\s+/gm, '');
  // Remove special chars
  text = text.replace(/[()\\[\\]{}\~_|>#]/g, '');
  // Expand common patterns
  text = text.replace(/(\\d+)\\s*°\\s*([CFcf])/g, (m, n, u) => n + ' degrees ' + (u.toUpperCase() === 'F' ? 'Fahrenheit' : 'Celsius'));
  text = text.replace(/(\\d+)%/g, '$1 percent');
  text = text.replace(/e\\.g\\./g, 'for example');
  text = text.replace(/i\\.e\\./g, 'that is');
  text = text.replace(/etc\\./g, 'etcetera');
  text = text.replace(/km\\/h/g, 'kilometers per hour');
  text = text.replace(/mph/g, 'miles per hour');
  // Newlines to sentence breaks
  text = text.replace(/\\n+/g, '. ');
  // Strip non-ASCII non-letter chars (emojis etc)
  text = text.replace(/[^\\x00-\\x7F]/g, (c) => {
    // Keep common Latin extended but strip emoji range (U+2600+)
    if (c.charCodeAt(0) >= 0x2600) return '';
    return c;
  });
  // Collapse whitespace
  text = text.replace(/\\s+/g, ' ');
  text = text.replace(/[\\.\\s]*\\.\\s*\\./g, '.');
  return text.trim();
}

// --- TTS ---

ttsToggle.addEventListener('click', () => {
  ttsEnabled = !ttsEnabled;
  ttsToggle.classList.toggle('active', ttsEnabled);
});

async function speakText(text) {
  if (!ttsEnabled || !text) return;
  const clean = sanitizeForTTS(text);
  if (!clean) return;
  try {
    const resp = await fetch(BASE + '/v1/audio/speak', {
      method: 'POST',
      headers: { ...authHeaders, 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ text: clean.slice(0, 5000) })
    });
    if (!resp.ok) return;
    const blob = await resp.blob();
    const url = URL.createObjectURL(blob);
    const audio = new Audio(url);
    audio.onended = () => { URL.revokeObjectURL(url); resumeListening(); };
    pauseListening();
    audio.play();
  } catch {}
}

// --- Wake Word Detection (mirrors tui/voice.go) ---

const WAKE_WORD = 'cedric';
const WAKE_PREFIXES = ['hey', 'hey,', 'okay', 'okay,', 'ok', 'ok,', 'yo', 'yo,', 'hi', 'hi,', 'hello', 'hello,'];
const SEPARATORS = [' ', ', ', '. ', '! ', '? '];

function extractAfterWakeWord(text) {
  const lower = text.toLowerCase().trim();

  // Try with greeting prefix
  for (const gp of WAKE_PREFIXES) {
    const full = gp + ' ' + WAKE_WORD;
    const result = matchWakePrefix(lower, text, full);
    if (result !== null) return result;
  }

  // Try bare wake word
  const result = matchWakePrefix(lower, text, WAKE_WORD);
  if (result !== null) return result;

  return null; // no wake word
}

function matchWakePrefix(lower, original, prefix) {
  for (const sep of SEPARATORS) {
    const full = prefix + sep;
    if (lower.startsWith(full)) {
      return original.slice(full.length).trim();
    }
  }
  // Exact match (wake word alone)
  const trimmed = lower.replace(/[.,!? ]+$/, '');
  if (trimmed === prefix) return '';
  return null;
}

// --- Hot Mic / VAD ---

voiceToggle.addEventListener('click', async () => {
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    addMsg('error', 'Microphone requires HTTPS. Access via https:// or localhost.');
    return;
  }
  voiceEnabled = !voiceEnabled;
  voiceToggle.classList.toggle('active', voiceEnabled);
  if (voiceEnabled) {
    // Also enable TTS when voice is on
    if (!ttsEnabled) {
      ttsEnabled = true;
      ttsToggle.classList.add('active');
    }
    await startVoiceListening();
  } else {
    stopVoiceListening();
  }
});

async function startVoiceListening() {
  try {
    audioStream = await navigator.mediaDevices.getUserMedia({
      audio: { sampleRate: SAMPLE_RATE, channelCount: 1, echoCancellation: true, noiseSuppression: true }
    });
    audioContext = new AudioContext({ sampleRate: SAMPLE_RATE });
    const source = audioContext.createMediaStreamSource(audioStream);
    analyser = audioContext.createAnalyser();
    analyser.fftSize = 2048;
    source.connect(analyser);

    // ScriptProcessor for raw PCM access
    const chunkSamples = Math.floor(SAMPLE_RATE * CHUNK_MS / 1000);
    voiceProcessor = audioContext.createScriptProcessor(chunkSamples, 1, 1);
    speechBuffer = [];
    silenceCount = 0;
    isSpeaking = false;
    leadInRing = [];

    voiceProcessor.onaudioprocess = handleAudioProcess;

    source.connect(voiceProcessor);
    voiceProcessor.connect(audioContext.destination);

    setVoiceStatus('listening');
    addMsg('system', 'Voice mode enabled \u2014 say "Cedric" followed by your message');
  } catch (err) {
    addMsg('error', 'Mic error: ' + err.message);
    voiceEnabled = false;
    voiceToggle.classList.remove('active');
  }
}

function stopVoiceListening() {
  if (voiceProcessor) { voiceProcessor.disconnect(); voiceProcessor = null; }
  if (audioContext) { audioContext.close(); audioContext = null; }
  if (audioStream) { audioStream.getTracks().forEach(t => t.stop()); audioStream = null; }
  speechBuffer = [];
  isSpeaking = false;
  setVoiceStatus('');
  addMsg('system', 'Voice mode disabled');
}

function pauseListening() {
  // Pause during TTS playback to avoid feedback
  if (voiceProcessor) voiceProcessor.onaudioprocess = null;
}

function resumeListening() {
  if (!voiceEnabled || !voiceProcessor) return;
  speechBuffer = [];
  isSpeaking = false;
  silenceCount = 0;
  leadInRing = [];
  voiceProcessor.onaudioprocess = handleAudioProcess;
  setVoiceStatus('listening');
}

// Extract handler for reuse
function handleAudioProcess(e) {
  if (!voiceEnabled || sending) return;
  const data = e.inputBuffer.getChannelData(0);
  const chunk = new Float32Array(data);

  let sum = 0;
  for (let i = 0; i < chunk.length; i++) sum += chunk[i] * chunk[i];
  const rms = Math.sqrt(sum / chunk.length);

  if (rms > RMS_THRESHOLD) {
    if (!isSpeaking) {
      isSpeaking = true;
      speechBuffer = [];
      for (const lb of leadInRing) speechBuffer.push(lb);
      setVoiceStatus('listening');
    }
    silenceCount = 0;
    speechBuffer.push(chunk);
  } else if (isSpeaking) {
    silenceCount++;
    speechBuffer.push(chunk);
    if (silenceCount >= SILENCE_CHUNKS) {
      const totalMs = (speechBuffer.length * CHUNK_MS);
      if (totalMs >= MIN_SPEECH_MS) {
        processVoiceSegment(speechBuffer);
      }
      speechBuffer = [];
      isSpeaking = false;
      silenceCount = 0;
    }
  }

  leadInRing.push(chunk);
  if (leadInRing.length > LEAD_IN_CHUNKS) leadInRing.shift();
}

async function processVoiceSegment(chunks) {
  setVoiceStatus('processing');

  // Concatenate float32 chunks and convert to int16 PCM
  let totalLen = 0;
  for (const c of chunks) totalLen += c.length;
  const pcm = new Int16Array(totalLen);
  let offset = 0;
  for (const c of chunks) {
    for (let i = 0; i < c.length; i++) {
      const s = Math.max(-1, Math.min(1, c[i]));
      pcm[offset++] = s < 0 ? s * 0x8000 : s * 0x7FFF;
    }
  }

  try {
    const resp = await fetch(BASE + '/v1/audio/transcribe', {
      method: 'POST',
      headers: { ...authHeaders, 'Content-Type': 'application/octet-stream' },
      credentials: 'same-origin',
      body: pcm.buffer
    });
    if (!resp.ok) { setVoiceStatus('listening'); return; }
    const data = await resp.json();
    const text = (data.text || '').trim();
    if (!text) { setVoiceStatus('listening'); return; }

    // Check for wake word
    const afterWake = extractAfterWakeWord(text);
    if (afterWake === null) {
      // No wake word — ignore
      setVoiceStatus('listening');
      return;
    }
    if (afterWake === '') {
      // Just the wake word — acknowledge
      addMsg('system', 'Listening...');
      setVoiceStatus('listening');
      return;
    }

    // Wake word + command — submit
    setVoiceStatus('');
    input.value = afterWake;
    sendMessage();
  } catch {
    setVoiceStatus('listening');
  }
}

function setVoiceStatus(state) {
  voiceStatus.className = '';
  if (state === 'listening') {
    voiceStatus.textContent = 'Listening for "Cedric"...';
    voiceStatus.classList.add('listening');
  } else if (state === 'processing') {
    voiceStatus.textContent = 'Transcribing...';
    voiceStatus.classList.add('processing');
  } else {
    voiceStatus.textContent = '';
  }
}

// --- Chat ---

function addMsg(role, text) {
  const div = document.createElement('div');
  div.className = 'msg ' + role;
  div.textContent = text;
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
  return div;
}

function addTool(label, text) {
  const div = document.createElement('div');
  div.className = 'msg tool';
  const lbl = document.createElement('div');
  lbl.className = 'tool-label';
  lbl.textContent = label;
  div.appendChild(lbl);
  const content = document.createElement('div');
  content.textContent = text;
  div.appendChild(content);
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
}

async function sendMessage() {
  const text = input.value.trim();
  if (!text || sending) return;
  sending = true;
  sendBtn.disabled = true;
  input.value = '';
  input.style.height = 'auto';

  addMsg('user', text);

  const body = { message: text };
  if (sessionId) body.session_id = sessionId;
  const selectedModel = modelSelect.value;
  if (selectedModel) body.model = selectedModel;

  let assistantDiv = null;
  let assistantText = '';

  try {
    const resp = await fetch(BASE + '/v1/chat', {
      method: 'POST',
      headers: { ...authHeaders, 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify(body)
    });

    if (!resp.ok) {
      addMsg('error', 'Error: ' + resp.status + ' ' + resp.statusText);
      return;
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      const lines = buffer.split('\n');
      buffer = lines.pop();

      let eventType = '';
      for (const line of lines) {
        if (line.startsWith('event: ')) {
          eventType = line.slice(7).trim();
        } else if (line.startsWith('data: ')) {
          const raw = line.slice(6);
          let data;
          try { data = JSON.parse(raw); } catch { continue; }

          switch (eventType) {
            case 'token':
              if (!assistantDiv) {
                assistantDiv = addMsg('assistant', '');
                assistantText = '';
              }
              assistantText += data.text || '';
              assistantDiv.textContent = assistantText;
              container.scrollTop = container.scrollHeight;
              break;
            case 'tool_call':
              addTool('calling: ' + (data.name || ''), '');
              break;
            case 'tool_result':
              addTool('result: ' + (data.name || ''), data.text || '');
              break;
            case 'done':
              if (data.session_id) sessionId = data.session_id;
              if (!assistantDiv && data.text) {
                assistantDiv = addMsg('assistant', data.text);
                assistantText = data.text;
              }
              speakText(assistantText);
              break;
            case 'error':
              addMsg('error', 'Error: ' + (data.text || 'unknown'));
              break;
          }
          eventType = '';
        }
      }
    }
  } catch (err) {
    addMsg('error', 'Connection error: ' + err.message);
  } finally {
    sending = false;
    sendBtn.disabled = false;
    if (!voiceEnabled) input.focus();
    else resumeListening();
  }
}

sendBtn.addEventListener('click', sendMessage);
input.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendMessage();
  }
});
input.addEventListener('input', () => {
  input.style.height = 'auto';
  input.style.height = Math.min(input.scrollHeight, 120) + 'px';
});

init();
</script>
</body>
</html>`

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(chatPageHTML))
}
