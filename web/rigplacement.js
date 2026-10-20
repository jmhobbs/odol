// Where the reference player rig stands in lodviewer.js. Plain math on
// {min: {x, y, z}, max: {x, y, z}} boxes (THREE.Box3 has that shape), with
// no imports, so it can be tested in Node without three.js.

// Gap between the model and the rig, as a fraction of the largest X/Z
// extent of either box. Same factor as viewer.js's rig.
const RIG_GAP_FRACTION = 0.25;

function center(min, max) {
  return (min + max) / 2;
}

// rigPlacement returns the translation that stands the rig beside the
// model: feet on the model's floor (internal/mlodbuild floors every built
// model at y=0), centered on the model's Z, and past its +X side by a gap,
// so the two boxes never overlap. With no model (modelBounds null) the rig
// stands centered on the origin.
export function rigPlacement(modelBounds, rigBounds) {
  const rigCenterX = center(rigBounds.min.x, rigBounds.max.x);
  const rigCenterZ = center(rigBounds.min.z, rigBounds.max.z);
  if (!modelBounds) {
    return { x: -rigCenterX, y: -rigBounds.min.y, z: -rigCenterZ };
  }

  const gap =
    RIG_GAP_FRACTION *
    Math.max(
      modelBounds.max.x - modelBounds.min.x,
      modelBounds.max.z - modelBounds.min.z,
      rigBounds.max.x - rigBounds.min.x,
      rigBounds.max.z - rigBounds.min.z
    );
  return {
    x: modelBounds.max.x + gap - rigBounds.min.x,
    y: modelBounds.min.y - rigBounds.min.y,
    z: center(modelBounds.min.z, modelBounds.max.z) - rigCenterZ,
  };
}
