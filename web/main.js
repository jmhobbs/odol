// Main-thread glue: spawns the conversion Worker, wires the file
// picker/buttons, drives Blob downloads, and renders a WebGL preview of
// the converted FBX. No parsing/conversion logic lives here - see
// worker.js and cmd/wasm/main.go. No WebGL logic lives here - see
// viewer.js.
import { initViewer, loadModel, clearModel, setCheckerEnabled } from './viewer.js';

const worker = new Worker('worker.js');

let nextRequestId = 1;
const pendingRequests = new Map();

const readyPromise = new Promise((resolve, reject) => {
  worker.onmessage = (event) => {
    const msg = event.data;

    if (msg.type === 'ready') {
      resolve();
      return;
    }
    if (msg.type === 'fatal') {
      reject(new Error(msg.error));
      return;
    }

    const pending = pendingRequests.get(msg.id);
    if (!pending) return;
    pendingRequests.delete(msg.id);
    if (msg.ok) {
      // Passed through whole: conversions carry {filename, data}, 'detect'
      // carries {family} instead - callers pick out what their action
      // produces.
      pending.resolve(msg);
    } else {
      pending.reject(new Error(msg.error));
    }
  };
  worker.onerror = (event) => reject(new Error(event.message));
});

function convert(action, name, arrayBuffer, ascii) {
  const id = nextRequestId++;
  return new Promise((resolve, reject) => {
    pendingRequests.set(id, { resolve, reject });
    worker.postMessage({ id, action, name, data: arrayBuffer, ascii }, [arrayBuffer]);
  });
}

function downloadBlob(filename, arrayBuffer) {
  const blob = new Blob([arrayBuffer]);
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

const fileInput = document.getElementById('file-input');
const statusEl = document.getElementById('status');
const buttons = {
  mlod: document.getElementById('btn-mlod'),
  fbxBinary: document.getElementById('btn-fbx-binary'),
};

function setStatus(text, isError) {
  statusEl.textContent = text;
  statusEl.className = isError ? 'error' : '';
}

// "Export MLOD + model.cfg" only works from an ODOL source (see
// internal/convert.ConvertToMLOD, which rejects anything else) - mlodAllowed
// tracks whether the currently selected file has been confirmed ODOL, and
// stays false (button disabled) until detection says otherwise. FBX export
// has no such restriction (it accepts both ODOL and MLOD input).
let busy = false;
let mlodAllowed = false;

function refreshButtonStates() {
  const hasFile = fileInput.files.length > 0;
  buttons.fbxBinary.disabled = busy || !hasFile;
  buttons.mlod.disabled = busy || !hasFile || !mlodAllowed;
}

// WebGL may be unavailable (old browser, disabled in settings, headless
// environment without a GPU). The viewer is a bonus, not core
// functionality, so its failure shouldn't block conversion/download.
let viewerAvailable = true;
try {
  initViewer(document.getElementById('viewer'));
} catch (err) {
  viewerAvailable = false;
  document.getElementById('viewer-wrap').style.display = 'none';
  console.error('WebGL preview unavailable:', err);
}

// The binary FBX preview conversion is cached per selected file so
// clicking "Export FBX (binary)" afterward reuses it instead of running
// odolConvertToFBX a second time. previewToken guards against a stale
// preview (from a file that's since been replaced) landing after a newer
// selection.
let currentFile = null;
let fbxPreviewPromise = null;
let previewToken = 0;

function startFbxPreview(file, token) {
  fbxPreviewPromise = (async () => {
    await readyPromise;
    const arrayBuffer = await file.arrayBuffer();
    return convert('fbx', file.name, arrayBuffer, false);
  })();

  fbxPreviewPromise
    .then((result) => {
      if (token !== previewToken || !viewerAvailable) return;
      loadModel(result.data);
      setStatus('Loaded ' + file.name, false);
    })
    .catch((err) => {
      if (token !== previewToken) return;
      setStatus('Preview error: ' + err.message, true);
    });

  return fbxPreviewPromise;
}

// Only the first ~16 bytes are needed to identify the format (see
// internal/detector.Detect), so this sends a small slice rather than the
// whole file - unlike the FBX preview above, which needs the full bytes.
async function detectAndGateMlod(file, token) {
  try {
    await readyPromise;
    const header = await file.slice(0, 16).arrayBuffer();
    const result = await convert('detect', file.name, header);
    if (token !== previewToken) return;
    mlodAllowed = result.family === 'ODOL';
  } catch {
    if (token !== previewToken) return;
    mlodAllowed = false;
  }
  if (token === previewToken) refreshButtonStates();
}

// Checker toggle only changes what the *next* loadModel() call renders
// (see viewer.js), so a toggle while a model is already showing needs to
// explicitly re-run loadModel from the cached preview bytes to take
// effect immediately.
const checkerToggle = document.getElementById('checker-toggle');
checkerToggle.addEventListener('change', async () => {
  setCheckerEnabled(checkerToggle.checked);
  if (!viewerAvailable || !fbxPreviewPromise) return;
  try {
    const result = await fbxPreviewPromise;
    loadModel(result.data);
  } catch {
    // Preview already failed and was reported by startFbxPreview - no
    // model to re-render.
  }
});

// Buttons start disabled (see index.html) and only become usable once a
// file is chosen (and, for MLOD, confirmed ODOL - see detectAndGateMlod).
fileInput.addEventListener('change', () => {
  const hasFile = fileInput.files.length > 0;

  previewToken++;
  currentFile = hasFile ? fileInput.files[0] : null;
  fbxPreviewPromise = null;
  mlodAllowed = false;
  refreshButtonStates();
  if (viewerAvailable) clearModel();

  if (currentFile) {
    setStatus('Rendering preview of ' + currentFile.name + ' ...', false);
    if (viewerAvailable) startFbxPreview(currentFile, previewToken);
    detectAndGateMlod(currentFile, previewToken);
  } else {
    setStatus('', false);
  }
});

async function handleClick(action, ascii) {
  const file = fileInput.files[0];
  if (!file) {
    setStatus('Choose a .p3d file first.', true);
    return;
  }

  busy = true;
  refreshButtonStates();
  try {
    await readyPromise;

    let result;
    const reusePreview = action === 'fbx' && !ascii && file === currentFile && fbxPreviewPromise;
    if (reusePreview) {
      setStatus('Preparing download of ' + file.name + ' ...', false);
      result = await fbxPreviewPromise;
    } else {
      setStatus('Converting ' + file.name + ' ...', false);
      const arrayBuffer = await file.arrayBuffer();
      result = await convert(action, file.name, arrayBuffer, ascii);
    }

    downloadBlob(result.filename, result.data);
    setStatus('Downloaded ' + result.filename, false);
  } catch (err) {
    setStatus('Error: ' + err.message, true);
  } finally {
    busy = false;
    refreshButtonStates();
  }
}

buttons.mlod.addEventListener('click', () => handleClick('mlod'));
buttons.fbxBinary.addEventListener('click', () => handleClick('fbx', false));

readyPromise.catch((err) => setStatus('Failed to load converter: ' + err.message, true));
