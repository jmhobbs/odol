// WebGL preview of every LOD odolFBXToP3D builds (see fbxToP3D in
// cmd/wasm/main.go and internal/fbx2p3d/preview.go). Each LOD arrives as
// flat positions/triangles typed arrays in P3D axes and meters, already
// rewound to WebGL's counter-clockwise front faces, so this module only
// wraps them in three.js objects. The only FBX parsed here is the
// reference player rig (see setRig).
//
// One LOD shows at a time. The highest-detail graphical LOD is also drawn
// as a faint ghost behind it, so a lower LOD or a collision box can be
// judged against the full model.
import * as THREE from 'three';
import { FBXLoader } from 'three/addons/loaders/FBXLoader.js';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { rigPlacement } from './rigplacement.js';

const BACKGROUND_COLOR = 0x22252a;
const GRAPHICAL_COLOR = 0xb8bcc4;
const WIREFRAME_COLOR = 0x1d1f24;
const MEMORY_POINT_COLOR = 0x7ee07e;
const MEMORY_POINT_SIZE_PX = 8;
const GHOST_COLOR = 0x9fd0ff;
const GHOST_OPACITY = 0.15;

// Collision LODs get their own tint so they're told apart at a glance.
// Keyed by the LOD names internal/inspect.ResolutionName returns.
const COLLISION_COLORS = {
  'Geometry': 0xe0a030,
  'View Geometry': 0x40b0e0,
  'Fire Geometry': 0xe05050,
};
const COLLISION_OPACITY = 0.35;

// The rig gets a color no LOD uses, so it never reads as part of the model.
const RIG_COLOR = 0xa57de0;
// playerRig_*.fbx are Blender exports in centimeters (see viewer.js
// loadRig). The preview is in meters.
const RIG_CENTIMETERS_TO_METERS = 0.01;
// FBXLoader asks for any texture the rig's materials name. Those
// materials are replaced in setRig, so every such URL gets this 1x1 grey
// PNG instead of a network request.
const PLACEHOLDER_TEXTURE_URL =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAAXNSR0IB2cksfwAAAARnQU1BAACxjwv8YQUAAAAgY0hSTQAAeiYAAICEAAD6AAAAgOgAAHUwAADqYAAAOpgAABdwnLpRPAAAAAlwSFlzAAAuIwAALiMBeKU/dgAAAAxJREFUCNdj2LNnDwAEbAI1ss8j0wAAAABJRU5ErkJggg==';

// createLodViewer renders into canvas and returns the controls the page
// drives. All state lives in this closure, so a page could hold more than
// one viewer.
export function createLodViewer(canvas) {
  // preserveDrawingBuffer: rendering is on-demand (see requestFrame), so
  // the canvas must keep its last frame between renders.
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, preserveDrawingBuffer: true });
  renderer.setPixelRatio(window.devicePixelRatio || 1);

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(BACKGROUND_COLOR);

  const camera = new THREE.PerspectiveCamera(50, 1, 0.01, 10000);
  const controls = new OrbitControls(camera, renderer.domElement);
  controls.enableDamping = true;

  // Same light directions as viewer.js, at lower intensities: viewer.js's
  // rig is tuned for FBXLoader's materials and washes out the flat-shaded
  // MeshStandardMaterial used here.
  scene.add(new THREE.AmbientLight(0xffffff, 0.6));
  scene.add(new THREE.HemisphereLight(0xffffff, 0x3a3a3a, 1.2));
  const key = new THREE.DirectionalLight(0xffffff, 2.5);
  key.position.set(1, 2, 1.5);
  scene.add(key);
  const fill = new THREE.DirectionalLight(0xffffff, 1);
  fill.position.set(-1.5, -0.5, -1);
  scene.add(fill);

  let frameRequested = false;
  let lodObjects = [];
  let wireframeObjects = [];
  let ghostObject = null;
  let highestDetailIndex = -1;
  let selectedIndex = -1;
  let ghostVisible = true;
  let wireframeVisible = false;
  // modelBounds is the union of every LOD with triangles, or null with no
  // LODs. The rig stands beside it.
  let modelBounds = null;
  let rigObject = null;
  let rigVisible = false;

  const rigLoadingManager = new THREE.LoadingManager();
  rigLoadingManager.setURLModifier(() => PLACEHOLDER_TEXTURE_URL);

  function requestFrame() {
    if (frameRequested) return;
    frameRequested = true;
    requestAnimationFrame(() => {
      frameRequested = false;
      controls.update();
      renderer.render(scene, camera);
    });
  }
  controls.addEventListener('change', requestFrame);

  function resize() {
    const width = canvas.clientWidth || 1;
    const height = canvas.clientHeight || 1;
    renderer.setSize(width, height, false);
    camera.aspect = width / height;
    camera.updateProjectionMatrix();
    requestFrame();
  }
  new ResizeObserver(resize).observe(canvas);
  resize();

  function updateVisibility() {
    lodObjects.forEach((object, i) => {
      object.visible = i === selectedIndex;
    });
    wireframeObjects.forEach((object, i) => {
      if (object) object.visible = wireframeVisible && i === selectedIndex;
    });
    if (ghostObject) {
      ghostObject.visible = ghostVisible && selectedIndex !== highestDetailIndex;
    }
    requestFrame();
  }

  // setLODs replaces every shown LOD with preview's (fbxToP3D's preview
  // object) and keeps the current selection if that index still exists.
  // reframe moves the camera to fit the model; leave it off for rebuilds
  // that don't change geometry, so the user's view is kept.
  function setLODs(preview, { reframe }) {
    clearObjects();

    for (const lod of preview.lods) {
      const geometry = meshGeometry(lod);
      const object = lodObject(lod, geometry);
      object.visible = false;
      scene.add(object);
      lodObjects.push(object);

      let wireframe = null;
      if (lod.triangles.length > 0) {
        wireframe = new THREE.LineSegments(
          new THREE.WireframeGeometry(geometry),
          new THREE.LineBasicMaterial({ color: WIREFRAME_COLOR })
        );
        wireframe.visible = false;
        scene.add(wireframe);
      }
      wireframeObjects.push(wireframe);
    }

    highestDetailIndex = preview.highestDetailIndex;
    if (highestDetailIndex >= 0) {
      // Shares the LOD's own geometry; clearObjects disposes it once.
      ghostObject = new THREE.Mesh(
        lodObjects[highestDetailIndex].geometry,
        new THREE.MeshBasicMaterial({
          color: GHOST_COLOR,
          transparent: true,
          opacity: GHOST_OPACITY,
          depthWrite: false,
          side: THREE.DoubleSide,
        })
      );
      scene.add(ghostObject);
    }

    if (selectedIndex < 0 || selectedIndex >= lodObjects.length) {
      selectedIndex = Math.max(highestDetailIndex, 0);
    }
    modelBounds = measureModel(preview);
    placeRig();
    if (reframe) frameCamera();
    updateVisibility();
  }

  function select(index) {
    selectedIndex = index;
    updateVisibility();
  }

  function setGhostVisible(visible) {
    ghostVisible = visible;
    updateVisibility();
  }

  function setWireframeVisible(visible) {
    wireframeVisible = visible;
    updateVisibility();
  }

  // setRig parses the reference rig FBX (playerRig_m.fbx), scales it to
  // meters, colors it RIG_COLOR and stands it beside the model. It shows
  // only while setRigVisible(true) is in effect.
  function setRig(fbxArrayBuffer) {
    if (rigObject) {
      scene.remove(rigObject);
      disposeTree(rigObject);
    }
    rigObject = new FBXLoader(rigLoadingManager).parse(fbxArrayBuffer, '');
    rigObject.scale.setScalar(RIG_CENTIMETERS_TO_METERS);
    const rigMaterial = new THREE.MeshStandardMaterial({ color: RIG_COLOR, roughness: 0.8 });
    rigObject.traverse((child) => {
      if (!child.isMesh) return;
      disposeMaterials(child.material);
      child.material = rigMaterial;
    });
    rigObject.visible = rigVisible;
    scene.add(rigObject);
    placeRig();
    if (rigVisible) frameCamera();
    requestFrame();
  }

  // setRigVisible shows or hides the rig and refits the camera to what is
  // shown, so the rig is in view when it appears.
  function setRigVisible(visible) {
    rigVisible = visible;
    if (rigObject) rigObject.visible = visible;
    frameCamera();
    requestFrame();
  }

  function clear() {
    clearObjects();
    selectedIndex = -1;
    modelBounds = null;
    placeRig();
    requestFrame();
  }

  // placeRig stands the rig beside modelBounds (see rigPlacement). It
  // measures the rig at the origin first, since rigPlacement works on the
  // rig's own box.
  function placeRig() {
    if (!rigObject) return;
    rigObject.position.set(0, 0, 0);
    rigObject.updateMatrixWorld(true);
    const rigBounds = new THREE.Box3().setFromObject(rigObject);
    if (rigBounds.isEmpty()) return;
    const { x, y, z } = rigPlacement(modelBounds, rigBounds);
    rigObject.position.set(x, y, z);
    rigObject.updateMatrixWorld(true);
    requestFrame();
  }

  function clearObjects() {
    const geometries = new Set();
    const materials = new Set();
    for (const object of [...lodObjects, ...wireframeObjects, ghostObject]) {
      if (!object) continue;
      scene.remove(object);
      geometries.add(object.geometry);
      materials.add(object.material);
      for (const child of object.children) {
        geometries.add(child.geometry);
        materials.add(child.material);
      }
    }
    geometries.forEach((geometry) => geometry.dispose());
    materials.forEach((material) => material.dispose());
    lodObjects = [];
    wireframeObjects = [];
    ghostObject = null;
    highestDetailIndex = -1;
  }

  // measureModel returns the union of the graphical and collision LODs'
  // boxes, or null if there are none. Memory points are left out: invview
  // sits well in front of the model by design (see
  // internal/mlodbuild.memoryLOD) and would push the camera far back and
  // the rig far off to the side.
  function measureModel(preview) {
    const box = new THREE.Box3();
    preview.lods.forEach((lod, i) => {
      if (lod.triangles.length === 0) return;
      const geometry = lodObjects[i].geometry;
      geometry.computeBoundingBox();
      box.union(geometry.boundingBox);
    });
    return box.isEmpty() ? null : box;
  }

  // frameCamera fits the model and, while it is shown, the rig.
  function frameCamera() {
    const box = new THREE.Box3();
    if (modelBounds) box.union(modelBounds);
    if (rigObject && rigVisible) box.union(new THREE.Box3().setFromObject(rigObject));
    if (box.isEmpty()) return;

    const size = box.getSize(new THREE.Vector3());
    const center = box.getCenter(new THREE.Vector3());
    const maxDim = Math.max(size.x, size.y, size.z) || 1;

    camera.near = maxDim / 100;
    camera.far = maxDim * 100;
    camera.updateProjectionMatrix();

    const fitDistance = (maxDim / 2 / Math.tan((camera.fov * Math.PI) / 360)) * 1.5;
    camera.position.set(center.x + fitDistance, center.y + fitDistance * 0.6, center.z + fitDistance);
    controls.target.copy(center);
    controls.update();
  }

  return { setLODs, select, setGhostVisible, setWireframeVisible, setRig, setRigVisible, clear };
}

function disposeMaterials(material) {
  const materials = Array.isArray(material) ? material : [material];
  for (const each of materials) {
    for (const value of Object.values(each)) {
      if (value && value.isTexture) value.dispose();
    }
    each.dispose();
  }
}

// disposeTree frees every geometry and material under root. The rig's
// meshes share one material, so it is collected once.
function disposeTree(root) {
  const materials = new Set();
  root.traverse((child) => {
    if (child.geometry) child.geometry.dispose();
    if (child.material) materials.add(child.material);
  });
  materials.forEach(disposeMaterials);
}

function meshGeometry(lod) {
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute('position', new THREE.BufferAttribute(lod.positions, 3));
  if (lod.triangles.length > 0) {
    geometry.setIndex(new THREE.BufferAttribute(lod.triangles, 1));
  }
  return geometry;
}

// lodObject picks how to draw one LOD:
// - no triangles (the Memory LOD): one dot per point
// - collision LODs: a tinted, half-transparent solid plus its hard edges
// - graphical LODs: a solid, flat-shaded grey mesh
// Solids draw front faces only, so a face with the wrong winding shows up
// as a hole instead of being hidden.
function lodObject(lod, geometry) {
  if (lod.triangles.length === 0) {
    return new THREE.Points(
      geometry,
      new THREE.PointsMaterial({ color: MEMORY_POINT_COLOR, size: MEMORY_POINT_SIZE_PX, sizeAttenuation: false })
    );
  }

  const collisionColor = COLLISION_COLORS[lod.name];
  if (collisionColor !== undefined) {
    const solid = new THREE.Mesh(
      geometry,
      new THREE.MeshStandardMaterial({
        color: collisionColor,
        flatShading: true,
        transparent: true,
        opacity: COLLISION_OPACITY,
        depthWrite: false,
      })
    );
    solid.add(new THREE.LineSegments(new THREE.EdgesGeometry(geometry), new THREE.LineBasicMaterial({ color: collisionColor })));
    return solid;
  }

  return new THREE.Mesh(
    geometry,
    new THREE.MeshStandardMaterial({ color: GRAPHICAL_COLOR, flatShading: true, roughness: 0.8 })
  );
}
