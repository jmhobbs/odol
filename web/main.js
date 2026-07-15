// Main-thread glue: spawns the conversion Worker, wires the file
// picker/buttons, and drives Blob downloads. No parsing/conversion logic
// lives here - see worker.js and cmd/wasm/main.go.
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
      pending.resolve({ filename: msg.filename, data: msg.data });
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

function setButtonsDisabled(disabled) {
  for (const button of Object.values(buttons)) {
    button.disabled = disabled;
  }
}

// Buttons start disabled (see index.html) and only become usable once a
// file is chosen.
fileInput.addEventListener('change', () => {
  setButtonsDisabled(fileInput.files.length === 0);
});

async function handleClick(action, ascii) {
  const file = fileInput.files[0];
  if (!file) {
    setStatus('Choose a .p3d file first.', true);
    return;
  }

  setStatus('Converting ' + file.name + ' ...', false);
  setButtonsDisabled(true);
  try {
    await readyPromise;
    const arrayBuffer = await file.arrayBuffer();
    const result = await convert(action, file.name, arrayBuffer, ascii);
    downloadBlob(result.filename, result.data);
    setStatus('Downloaded ' + result.filename, false);
  } catch (err) {
    setStatus('Error: ' + err.message, true);
  } finally {
    setButtonsDisabled(false);
  }
}

buttons.mlod.addEventListener('click', () => handleClick('mlod'));
buttons.fbxBinary.addEventListener('click', () => handleClick('fbx', false));

readyPromise.catch((err) => setStatus('Failed to load converter: ' + err.message, true));
