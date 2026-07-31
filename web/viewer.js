// WebGL preview of a converted FBX buffer, via vendored three.js (see
// vendor/three/README.md - no CDN, no bundler). internal/fbxexport's
// materials carry a real diffuse color (a light gray) and a texture
// *path* string, but no embedded pixels (DayZ .paa source textures
// aren't extractable here) - so what actually renders is that gray,
// modulated by whatever placeholder texture we hand back below.
//
// FBXLoader tries to resolve those texture path strings as URLs. Left
// alone that fires real (failing) network requests for files that don't
// exist anywhere reachable. A LoadingManager.setURLModifier stub
// intercepts every such URL and hands back a local placeholder instead,
// keeping the "no network requests" guarantee the conversion path already
// has (see plan_fbx-wasm-browser-export.md).
import * as THREE from 'three';
import { FBXLoader } from 'three/addons/loaders/FBXLoader.js';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';

// 20x20 black/white checkerboard, 10px squares. Tiled across each
// surface (see applyCheckerTiling) so curvature, scale, and UV seams are
// visible on models that otherwise have no real texture data. Must have
// no fully-black-times-transparent pixels: a map's RGB is multiplied
// into the material's diffuse color in the shader, so a bad placeholder
// can crush the whole material to black regardless of the light rig -
// the black squares here are deliberate, opaque, and expected to read as
// genuinely dark.
const CHECKER_TEXTURE_URL =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAIAAAD/gAIDAAAAAXNSR0IB2cksfwAAAARnQU1BAACxjwv8YQUAAAAgY0hSTQAAeiYAAICEAAD6AAAAgOgAAHUwAADqYAAAOpgAABdwnLpRPAAAAAlwSFlzAAAuIwAALiMBeKU/dgAAAKdJREFUeNrt00ENADAIBMG2ItCC/xcq0FILfdJkVsFlAjsi1ryqauCqswQLFixYsGAJFixYsGDBEixYsGDBgiVYsGDBggVLsGDBggULlmDBggULFizBggULFixYggULFixYsPTc7u6BszLTZXlDWIIFCxYsWLAECxYsWLBgCRYsWLBgwRIsWLBgwYIlWLBgwYIFS7BgwYIFC5ZgwYIFCxYswYIFC9bfXeOxBqNby9AMAAAAAElFTkSuQmCC';

const CHECKER_REPEAT = 8;

// Opaque grey 1x1 pixel - the "checker off" placeholder. Must stay
// opaque grey, not transparent black: see the multiplication note
// above.
const GREY_TEXTURE_URL =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAAXNSR0IB2cksfwAAAARnQU1BAACxjwv8YQUAAAAgY0hSTQAAeiYAAICEAAD6AAAAgOgAAHUwAADqYAAAOpgAABdwnLpRPAAAAAlwSFlzAAAuIwAALiMBeKU/dgAAAAxJREFUCNdj2LNnDwAEbAI1ss8j0wAAAABJRU5ErkJggg==';


let renderer, scene, camera, controls, canvas;
let currentModel = null;
let currentRig = null;
let frameRequested = false;
let checkerEnabled = false;

const loaderManager = new THREE.LoadingManager();
loaderManager.setURLModifier(() => (checkerEnabled ? CHECKER_TEXTURE_URL : GREY_TEXTURE_URL));
// The placeholder texture above still decodes asynchronously (it's an
// <img src="data:..."> under the hood). The render triggered right after
// parse() can land before that decode finishes, uploading a blank/black
// GPU texture - and since rendering is on-demand (see requestFrame),
// nothing repaints afterward to pick up the real pixels. Re-render once
// the manager's queue (including that decode) actually drains.
loaderManager.onLoad = () => requestFrame();

// setCheckerEnabled only affects textures assigned on the *next*
// loadModel() call - the caller is responsible for re-parsing the
// currently displayed model (from its cached FBX bytes) if it wants the
// change to apply immediately. Kept that way so viewer.js doesn't need
// to hold onto raw FBX bytes it otherwise has no use for.
export function setCheckerEnabled(enabled) {
  checkerEnabled = enabled;
}

export function initViewer(canvasEl) {
  canvas = canvasEl;

  // preserveDrawingBuffer is required because we render on-demand (only
  // on load/resize/control-change) rather than every frame - without it
  // the browser auto-clears the drawing buffer after each composite and
  // the canvas goes blank on the very next repaint.
  renderer = new THREE.WebGLRenderer({ canvas, antialias: true, preserveDrawingBuffer: true });
  renderer.setPixelRatio(window.devicePixelRatio || 1);

  scene = new THREE.Scene();
  scene.background = new THREE.Color(0x22252a);

  camera = new THREE.PerspectiveCamera(50, 1, 0.01, 10000);

  controls = new OrbitControls(camera, renderer.domElement);
  controls.enableDamping = true;
  controls.addEventListener('change', requestFrame);

  scene.add(new THREE.AmbientLight(0xffffff, 2));
  scene.add(new THREE.HemisphereLight(0xffffff, 0x3a3a3a, 3));
  const key = new THREE.DirectionalLight(0xffffff, 4);
  key.position.set(1, 2, 1.5);
  scene.add(key);
  const fill = new THREE.DirectionalLight(0xffffff, 2);
  fill.position.set(-1.5, -0.5, -1);
  scene.add(fill);

  resize();
  window.addEventListener('resize', resize);

  requestFrame();
}

function resize() {
  if (!renderer || !canvas) return;
  const width = canvas.clientWidth || 1;
  const height = canvas.clientHeight || 1;
  renderer.setSize(width, height, false);
  camera.aspect = width / height;
  camera.updateProjectionMatrix();
  requestFrame();
}

function requestFrame() {
  if (frameRequested) return;
  frameRequested = true;
  requestAnimationFrame(() => {
    frameRequested = false;
    controls.update();
    renderer.render(scene, camera);
  });
}

// loadModel parses fbxArrayBuffer (the same bytes the download button
// produces) and shows it, replacing and disposing any prior model.
export function loadModel(fbxArrayBuffer) {
  clearModel();
  const object = new FBXLoader(loaderManager).parse(fbxArrayBuffer, '');
  applyMapTiling(object);
  scene.add(object);
  currentModel = object;
  if (currentRig) positionRigNextToModel(currentRig);
  frameCameraToObjects([currentModel, currentRig]);
  requestFrame();
}

// loadRig parses a reference-rig FBX (playerRig_m.fbx/playerRig_f.fbx,
// fetched whole - unlike the P3D preview these are real FBX files, not
// wasm-converted output) and shows it alongside any currently-loaded P3D
// model. Independent of currentModel: swapping/clearing the P3D file does
// not affect the rig, and vice versa.
//
// No scale correction needed: playerRig_*.fbx are ordinary Blender
// exports (raw units = centimeters, ~180 units tall for a real human
// height) and the P3D->FBX converter (internal/fbxexport, see
// plan_fbx-realworld-scale.md) also outputs centimeters, so both land in
// the same raw-unit convention already.
export function loadRig(fbxArrayBuffer) {
  clearRig();
  const object = new FBXLoader(loaderManager).parse(fbxArrayBuffer, '');
  applyMapTiling(object);
  positionRigNextToModel(object);
  scene.add(object);
  currentRig = object;
  frameCameraToObjects([currentModel, currentRig]);
  requestFrame();
}

export function clearRig() {
  if (!currentRig) return;
  scene.remove(currentRig);
  disposeObject(currentRig);
  currentRig = null;
  frameCameraToObjects([currentModel, currentRig]);
  requestFrame();
}

// Positions rigObject relative to the world origin, which is also where a
// loaded P3D model's own origin sits (loadModel never translates it) -
// see plan_playerrig-alignment-transparency.md. Two independent things:
// - Y: centers the rig's bounding box on y=0 (half above, half below)
//   rather than leaving it feet-at-origin as authored, so a loaded
//   model's own origin/attach point lines up with the rig's vertical
//   midpoint instead of its feet. Unconditional - a property of the
//   rig's own placement, not the model's.
// - X: shifts the rig beside currentModel rather than overlapping it.
//   Falls back to no shift when there's no model (or either bounding box
//   is empty), so a rig selected before any P3D file is loaded still
//   renders sensibly, centered on Y, at the origin on X.
function positionRigNextToModel(rigObject) {
  rigObject.position.set(0, 0, 0);

  const rigBox = new THREE.Box3().setFromObject(rigObject);
  if (!rigBox.isEmpty()) {
    rigObject.position.y -= (rigBox.min.y + rigBox.max.y) / 2;
  }

  if (!currentModel) return;

  const modelBox = new THREE.Box3().setFromObject(currentModel);
  if (modelBox.isEmpty() || rigBox.isEmpty()) return;

  const modelSize = modelBox.getSize(new THREE.Vector3());
  const rigSize = rigBox.getSize(new THREE.Vector3());
  const gap = Math.max(modelSize.x, modelSize.z, rigSize.x, rigSize.z) * 0.25;

  // rigBox was measured before the Y shift above, but its X extents
  // (min.x/max.x) are unaffected by a pure Y translation, so it's still
  // valid here.
  rigObject.position.x += modelBox.max.x - rigBox.min.x + gap;
}

// By default a map's UVs span 0-1 across a whole mesh, so the checker
// texture would show as one giant 2x2 block instead of a grid. Repeating
// it turns it into an actual visual reference for scale/curvature/UV
// seams. A no-op visually when the plain white placeholder is active,
// but harmless to apply either way.
function applyMapTiling(object) {
  object.traverse((child) => {
    if (!child.isMesh) return;
    const materials = Array.isArray(child.material) ? child.material : [child.material];
    for (const material of materials) {
      if (!material.map) continue;
      material.map.wrapS = THREE.RepeatWrapping;
      material.map.wrapT = THREE.RepeatWrapping;
      material.map.repeat.set(CHECKER_REPEAT, CHECKER_REPEAT);
      material.map.needsUpdate = true;
    }
  });
}

export function clearModel() {
  if (!currentModel) return;
  scene.remove(currentModel);
  disposeObject(currentModel);
  currentModel = null;
  if (currentRig) positionRigNextToModel(currentRig);
  frameCameraToObjects([currentModel, currentRig]);
  requestFrame();
}

function disposeObject(root) {
  root.traverse((child) => {
    if (child.geometry) child.geometry.dispose();
    if (!child.material) return;
    const materials = Array.isArray(child.material) ? child.material : [child.material];
    for (const material of materials) {
      for (const value of Object.values(material)) {
        if (value && value.isTexture) value.dispose();
      }
      material.dispose();
    }
  });
}

// Frames the camera to the union of whatever's currently visible (P3D
// model and/or reference rig), so toggling either one keeps both in view
// rather than only ever fitting to the model.
function frameCameraToObjects(objects) {
  const box = new THREE.Box3();
  let hasContent = false;
  for (const object of objects) {
    if (!object) continue;
    const objectBox = new THREE.Box3().setFromObject(object);
    if (objectBox.isEmpty()) continue;
    box.union(objectBox);
    hasContent = true;
  }
  if (!hasContent) return;

  const size = box.getSize(new THREE.Vector3());
  const center = box.getCenter(new THREE.Vector3());
  const maxDim = Math.max(size.x, size.y, size.z) || 1;

  camera.near = maxDim / 100;
  camera.far = maxDim * 100;
  camera.updateProjectionMatrix();

  const fitDistance = (maxDim / 2 / Math.tan((camera.fov * Math.PI) / 360)) * 1.5;
  camera.position.set(
    center.x + fitDistance,
    center.y + fitDistance * 0.6,
    center.z + fitDistance
  );
  controls.target.copy(center);
  controls.update();
}
