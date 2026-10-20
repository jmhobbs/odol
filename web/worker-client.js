// Main-thread side of the worker.js protocol, shared by index.html and
// fbx2p3d.html: spawns the Worker, waits for its 'ready'/'fatal' message,
// and matches each response to its request by id.
export function createWorkerClient(url) {
  const worker = new Worker(url);

  let nextRequestId = 1;
  const pendingRequests = new Map();

  const ready = new Promise((resolve, reject) => {
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
        // Passed through whole: each action carries its own fields
        // (conversions carry {filename, data}, 'detect' carries {family},
        // and so on) - callers pick out what their action produces.
        pending.resolve(msg);
      } else {
        pending.reject(new Error(msg.error));
      }
    };
    worker.onerror = (event) => reject(new Error(event.message));
  });

  // request posts message (plus a fresh id) and resolves with the matching
  // response. transfer lists ArrayBuffers to hand over without copying;
  // they're detached on this side once posted.
  function request(message, transfer = []) {
    const id = nextRequestId++;
    return new Promise((resolve, reject) => {
      pendingRequests.set(id, { resolve, reject });
      worker.postMessage({ ...message, id }, transfer);
    });
  }

  return { ready, request };
}

// downloadBlob saves arrayBuffer as filename through a synthetic
// <a download> click.
export function downloadBlob(filename, arrayBuffer) {
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
