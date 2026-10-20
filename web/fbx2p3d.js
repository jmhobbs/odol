// Main-thread glue for fbx2p3d.html: the LOD rows and settings form,
// preview rebuilds, per-LOD stats, and the P3D download. The build itself
// runs in worker.js (odolFBXToP3D, see cmd/wasm/main.go and
// internal/fbx2p3d) and the 3D preview lives in lodviewer.js.
import { createWorkerClient, downloadBlob } from './worker-client.js';
import { createLodViewer } from './lodviewer.js';

// Long enough that typing a scale value doesn't rebuild on every
// keystroke.
const PREVIEW_DEBOUNCE_MS = 250;

// index.html serves the same rig for its "Reference Rig" option.
const RIG_URL = 'playerRig_m.fbx';

const workerClient = createWorkerClient('worker.js');

const lodTableBody = document.querySelector('#lod-table tbody');
const lodRowTemplate = document.getElementById('lod-row-template');
const addLODButton = document.getElementById('add-lod');
const textureInput = document.getElementById('texture');
const materialInput = document.getElementById('material');
const fireMaterialSelect = document.getElementById('fire-material');
const massInput = document.getElementById('mass');
const scaleInput = document.getElementById('scale');
const outputNameInput = document.getElementById('output-name');
const downloadButton = document.getElementById('download');
const statusEl = document.getElementById('status');
const warningsEl = document.getElementById('warnings');
const ghostToggle = document.getElementById('ghost-toggle');
const wireframeToggle = document.getElementById('wireframe-toggle');
const rigToggle = document.getElementById('rig-toggle');
const scaleHeight = document.getElementById('scale-height');
const scaleRatio = document.getElementById('scale-ratio');
const statsBody = document.querySelector('#lod-stats tbody');

function setStatus(text, isError) {
  statusEl.textContent = text;
  statusEl.className = isError ? 'error' : '';
}

function setWarnings(warnings) {
  warningsEl.replaceChildren(
    ...warnings.map((warning) => {
      const li = document.createElement('li');
      li.textContent = 'Warning: ' + warning;
      return li;
    })
  );
}

// WebGL may be unavailable (old browser, disabled in settings, headless
// environment without a GPU). Stats and download still work without it.
let viewer = null;
try {
  viewer = createLodViewer(document.getElementById('viewer'));
} catch (err) {
  document.getElementById('viewer-wrap').style.display = 'none';
  console.error('WebGL preview unavailable:', err);
}

// --- LOD rows ---

// readRows lists the LOD rows in table order. A row's resolution is its
// position (first row 1, next 2, and so on), so the most detailed LOD is
// always the first row. A row without a file keeps its position.
function readRows() {
  return Array.from(lodTableBody.rows, (row, index) => ({
    row,
    resolution: index + 1,
    file: row.querySelector('.fbx-file').files[0] || null,
  }));
}

// renumberRows shows each row's resolution and disables the moves that
// would leave the table.
function renumberRows() {
  const rows = Array.from(lodTableBody.rows);
  rows.forEach((row, index) => {
    row.querySelector('.resolution').textContent = index + 1;
    row.querySelector('.move-up').disabled = index === 0;
    row.querySelector('.move-down').disabled = index === rows.length - 1;
  });
}

// rowsChanged runs after any file pick, move or removal, since each can
// change which file has which resolution.
function rowsChanged() {
  renumberRows();
  updateOutputName();
  schedulePreview();
}

// addLODRow appends a row at the next resolution. An empty row changes no
// build, so it only renumbers and disables Download until a file is picked.
function addLODRow() {
  const row = lodRowTemplate.content.firstElementChild.cloneNode(true);

  row.querySelector('.fbx-file').addEventListener('change', rowsChanged);
  row.querySelector('.move-up').addEventListener('click', () => {
    lodTableBody.insertBefore(row, row.previousElementSibling);
    rowsChanged();
  });
  row.querySelector('.move-down').addEventListener('click', () => {
    lodTableBody.insertBefore(row.nextElementSibling, row);
    rowsChanged();
  });
  row.querySelector('.remove-lod').addEventListener('click', () => {
    row.remove();
    rowsChanged();
  });

  lodTableBody.appendChild(row);
  renumberRows();
  refreshDownloadButton();
}

// --- Output name ---

// Follows the CLI's default (the highest-detail LOD's FBX base name) until
// the user types their own.
let outputNameEdited = false;
outputNameInput.addEventListener('input', () => {
  outputNameEdited = outputNameInput.value.trim() !== '';
});

function updateOutputName() {
  if (outputNameEdited) return;
  const highestDetail = readRows().find((r) => r.file);
  outputNameInput.value = highestDetail ? highestDetail.file.name.replace(/\.[^.]*$/, '') + '.p3d' : '';
}

function outputName() {
  const name = outputNameInput.value.trim() || 'model.p3d';
  return name.toLowerCase().endsWith('.p3d') ? name : name + '.p3d';
}

// --- Builds ---

// buildRequest reads every row's file fresh: the buffers are transferred
// to the worker and detached here, so nothing is cached between builds.
async function buildRequest(rows) {
  const lods = await Promise.all(
    rows.map(async (r) => ({
      resolution: r.resolution,
      name: r.file.name,
      data: await r.file.arrayBuffer(),
    }))
  );
  return {
    request: {
      lods,
      texture: textureInput.value.trim(),
      material: materialInput.value.trim(),
      fireMaterial: fireMaterialSelect.value,
      mass: Number(massInput.value),
      scale: Number(scaleInput.value),
    },
    transfer: lods.map((lod) => lod.data),
  };
}

async function runBuild(rows) {
  await workerClient.ready;
  const { request, transfer } = await buildRequest(rows);
  return workerClient.request({ action: 'fbx2p3d', request }, transfer);
}

// previewToken drops a preview that lands after a newer one was asked for.
// framedGeometryKey tracks which files and scale the camera was last
// fitted to, so reordering LODs keeps the user's view.
let previewTimer = null;
let previewToken = 0;
let framedGeometryKey = null;

// geometryKey ignores row order: moving a LOD changes resolutions but not
// the model's overall bounds.
function geometryKey(rows) {
  const files = rows.map((r) => `${r.file.name}:${r.file.size}:${r.file.lastModified}`).sort();
  return [Number(scaleInput.value), ...files].join('|');
}

function schedulePreview() {
  refreshDownloadButton();
  clearTimeout(previewTimer);
  previewTimer = setTimeout(runPreview, PREVIEW_DEBOUNCE_MS);
}

async function runPreview() {
  const token = ++previewToken;
  const rows = readRows().filter((r) => r.file);
  if (rows.length === 0) {
    clearPreview();
    setStatus('', false);
    return;
  }

  setStatus('Building preview ...', false);
  try {
    const result = await runBuild(rows);
    if (token !== previewToken) return;

    const key = geometryKey(rows);
    showPreview(result.preview, key !== framedGeometryKey);
    framedGeometryKey = key;
    setWarnings(result.warnings);
    setStatus('', false);
  } catch (err) {
    if (token !== previewToken) return;
    clearPreview();
    setStatus('Error: ' + err.message, true);
  }
}

// --- Preview and stats ---

let selectedLODIndex = -1;

function showPreview(preview, reframe) {
  if (selectedLODIndex < 0 || selectedLODIndex >= preview.lods.length) {
    selectedLODIndex = Math.max(preview.highestDetailIndex, 0);
  }
  if (viewer) {
    viewer.setLODs(preview, { reframe });
    viewer.select(selectedLODIndex);
  }
  renderStats(preview);
  renderScaleReference(preview);
}

function clearPreview() {
  framedGeometryKey = null;
  selectedLODIndex = -1;
  if (viewer) viewer.clear();
  statsBody.replaceChildren();
  clearScaleReference();
  setWarnings([]);
}

function renderStats(preview) {
  statsBody.replaceChildren(
    ...preview.lods.map((lod, i) => {
      const tr = document.createElement('tr');
      if (i === selectedLODIndex) tr.className = 'selected';

      const nameCell = document.createElement('td');
      nameCell.textContent = lod.name;
      if (i === preview.highestDetailIndex) {
        const badge = document.createElement('span');
        badge.className = 'ghost-badge';
        badge.textContent = ' (ghost)';
        nameCell.appendChild(badge);
      }
      if (lod.selections.length > 0) {
        const selections = document.createElement('div');
        selections.className = 'selections';
        selections.textContent = lod.selections.join(', ');
        nameCell.appendChild(selections);
      }
      tr.appendChild(nameCell);

      for (const count of [lod.vertexCount, lod.faceCount, lod.triangleCount]) {
        const td = document.createElement('td');
        td.className = 'count';
        td.textContent = count.toLocaleString();
        tr.appendChild(td);
      }

      tr.addEventListener('click', () => {
        selectedLODIndex = i;
        for (const other of statsBody.rows) other.classList.toggle('selected', other === tr);
        if (viewer) viewer.select(i);
      });
      return tr;
    })
  );
}

// renderScaleReference shows the highest-detail LOD's height against the
// reference human, the same numbers the CLI's size preview prints.
function renderScaleReference(preview) {
  if (preview.highestDetailIndex < 0) {
    clearScaleReference();
    return;
  }
  const height = document.createElement('strong');
  height.textContent = `${preview.boundingSize.y.toFixed(4)} m`;
  scaleHeight.replaceChildren(height, ' tall');
  scaleRatio.textContent = `${preview.heightRatio.toPrecision(3)}x the reference human`;
}

function clearScaleReference() {
  scaleHeight.textContent = 'Add an FBX file to compare its height with a player.';
  scaleRatio.textContent = '';
}

ghostToggle.addEventListener('change', () => {
  if (viewer) viewer.setGhostVisible(ghostToggle.checked);
});
wireframeToggle.addEventListener('change', () => {
  if (viewer) viewer.setWireframeVisible(wireframeToggle.checked);
});

// --- Player rig ---

// rigLoading fetches and parses the rig once, on the first check of the
// box. A failed load is dropped so the next check tries again.
let rigLoading = null;

function loadRig() {
  if (!rigLoading) {
    rigLoading = fetch(RIG_URL)
      .then((response) => {
        if (!response.ok) throw new Error(`failed to fetch ${RIG_URL} (${response.status})`);
        return response.arrayBuffer();
      })
      .then((buffer) => viewer.setRig(buffer));
    rigLoading.catch(() => {
      rigLoading = null;
    });
  }
  return rigLoading;
}

// The box may be unchecked while the rig loads, so visibility follows the
// box's state once loading ends, not the state that started it.
rigToggle.disabled = !viewer;
rigToggle.addEventListener('change', async () => {
  if (rigToggle.checked) {
    try {
      await loadRig();
    } catch (err) {
      rigToggle.checked = false;
      setStatus('Player rig: ' + err.message, true);
    }
  }
  viewer.setRigVisible(rigToggle.checked);
});

// --- Download ---

let downloading = false;

function refreshDownloadButton() {
  const rows = readRows();
  downloadButton.disabled = downloading || rows.length === 0 || rows.some((r) => !r.file);
}

// Download builds again with the current form, rather than reusing the
// last preview, so texture, material, fire material and mass edits (which
// don't trigger a preview) are always included. Empty texture and material
// are allowed: a P3D face can have neither.
downloadButton.addEventListener('click', async () => {
  downloading = true;
  refreshDownloadButton();
  setStatus('Building P3D ...', false);
  try {
    const result = await runBuild(readRows());
    setWarnings(result.warnings);
    const filename = outputName();
    downloadBlob(filename, result.data);
    setStatus('Downloaded ' + filename, false);
  } catch (err) {
    setStatus('Error: ' + err.message, true);
  } finally {
    downloading = false;
    refreshDownloadButton();
  }
});

// --- Setup ---

// formatScale pads the scale to at least two decimals, so it reads as a
// float ("1.00") after typing or stepping. Extra decimals are kept: 0.0254
// (inches to meters) is a real scale factor.
function formatScale() {
  const text = scaleInput.value;
  const scale = Number(text);
  if (text === '' || !Number.isFinite(scale)) return;
  const decimals = (text.split('.')[1] || '').length;
  scaleInput.value = scale.toFixed(Math.max(2, decimals));
}

scaleInput.addEventListener('input', () => schedulePreview());
scaleInput.addEventListener('change', formatScale);
addLODButton.addEventListener('click', () => addLODRow());
addLODRow();
clearScaleReference();

workerClient.ready
  .then(() => workerClient.request({ action: 'penetrationMaterials' }))
  .then((result) => {
    for (const name of result.names) {
      const option = document.createElement('option');
      option.value = name;
      option.textContent = name;
      fireMaterialSelect.appendChild(option);
    }
  })
  .catch((err) => setStatus('Failed to load converter: ' + err.message, true));
