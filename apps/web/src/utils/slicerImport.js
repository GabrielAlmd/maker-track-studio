import api from '../services/api';
// Read summary comments only. Never execute G-code or infer quantities from moves.
export const MAX_SLICER_BYTES = 32 * 1024 * 1024;
const invalid = () => new Error('slicer.invalid');

export function parseSlicerGcode(text) {
  if (text.length > MAX_SLICER_BYTES || text.includes('\0')) throw invalid();
  let grams = null;
  let minutes = null;
  for (const line of text.split(/\r?\n/)) {
    const match = /^;\s*(filament used \[g\]|estimated printing time \(normal mode\))\s*=\s*(.*?)\s*$/.exec(line);
    if (!match) continue;
    if (match[1] === 'filament used [g]') {
      if (grams !== null) throw invalid();
      grams = match[2].split(',').map(value => {
        if (!/^\d+(?:\.\d+)?$/.test(value.trim())) throw invalid();
        const n = Number(value);
        if (!Number.isFinite(n) || n > 1e9) throw invalid();
        return n;
      });
      if (grams.length > 100) throw invalid();
    } else {
      if (minutes !== null || !/^(?:\d+d\s*)?(?:\d+h\s*)?(?:\d+m\s*)?(?:\d+s\s*)?$/.test(match[2]) || !match[2]) throw invalid();
      let seconds = 0;
      for (const [, value, unit] of match[2].matchAll(/(\d+)([dhms])/g)) seconds += Number(value) * ({ d: 86400, h: 3600, m: 60, s: 1 }[unit]);
      minutes = Math.ceil(seconds / 60);
      if (!Number.isSafeInteger(minutes) || minutes > 1e9) throw invalid();
    }
  }
  if (grams === null && minutes === null) throw invalid();
  return {
    estimated_time_minutes: minutes,
    // Zero slots are unused tools. Keep their indexes when labeling active tools.
    materials: (grams || []).flatMap((g, index) => g === 0 ? [] : [{ source_tool: index + 1, filament_id: '', weight_used: g, estimated_weight_g: g, waste_weight_g: null, measurement_state: 'estimated' }]),
  };
}

export async function readSlicerFiles(files) {
  if (!files.length || files.length > 16 || files.reduce((sum, file) => sum + file.size, 0) > 64 * 1024 * 1024) throw invalid();
  // Validate the complete batch before returning any rows to the form.
  const results = [];
  for (const file of files) {
    if (!/\.(gcode|3mf)$/i.test(file.name) || file.size > MAX_SLICER_BYTES) throw invalid();
    if (/\.3mf$/i.test(file.name)) {
      const form = new FormData();
      form.append('file', file);
      const { data } = await api.post('/v2/production/slicer/inspect', form, { headers: { 'Content-Type': 'multipart/form-data' } });
      results.push(...data.plates);
      continue;
    }
    results.push({ ...parseSlicerGcode(await file.text()), source_label: file.name });
  }
  return results;
}
