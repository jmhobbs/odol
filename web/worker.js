// Runs the odol WASM module off the main thread so a large model doesn't
// freeze the page's UI while it's being parsed/marshaled. Go's js/wasm
// target is single-threaded, so isolating it in a Worker is what actually
// buys responsiveness here - the Go side (cmd/wasm) just exposes plain
// synchronous functions.
importScripts('public/wasm_exec.js');

let ready = false;
const queued = [];

const go = new Go();
WebAssembly.instantiateStreaming(fetch('public/odol.wasm'), go.importObject)
  .then((result) => {
    // go.run() executes main() synchronously up to the point where it
    // parks (our main() registers the odolConvertTo* globals, then blocks
    // forever on `select {}` to keep the instance alive) - by the time
    // this call returns, the globals are already registered and safe to
    // call. It's still awaited (via .catch below) purely to surface an
    // unexpected exit as a fatal error.
    go.run(result.instance).catch((err) => {
      postMessage({ type: 'fatal', error: String(err) });
    });

    ready = true;
    postMessage({ type: 'ready' });
    for (const msg of queued.splice(0)) {
      handleConvert(msg);
    }
  })
  .catch((err) => {
    postMessage({ type: 'fatal', error: 'failed to load odol.wasm: ' + err });
  });

onmessage = (event) => {
  if (!ready) {
    queued.push(event.data);
    return;
  }
  handleConvert(event.data);
};

function handleConvert(msg) {
  const { id, action, name, data, ascii } = msg;
  let result;
  try {
    if (action === 'mlod') {
      result = odolConvertToMLOD(name, new Uint8Array(data));
    } else if (action === 'fbx') {
      result = odolConvertToFBX(name, new Uint8Array(data), !!ascii);
    } else if (action === 'detect') {
      result = odolDetectFormat(new Uint8Array(data));
    } else {
      result = { ok: false, error: 'unknown action: ' + action };
    }
  } catch (err) {
    result = { ok: false, error: String(err) };
  }

  if (!result.ok) {
    postMessage({ id, ok: false, error: result.error });
    return;
  }

  // 'detect' carries no binary payload - just the detected family - so
  // it skips the transferable-buffer path below.
  if (action === 'detect') {
    postMessage({ id, ok: true, family: result.family });
    return;
  }

  // result.data is a fresh Uint8Array allocated by the Go bridge; its
  // buffer is exactly-sized, so it can be transferred (zero-copy)
  // straight back to the main thread.
  postMessage(
    { id, ok: true, filename: result.filename, data: result.data.buffer },
    [result.data.buffer]
  );
}
