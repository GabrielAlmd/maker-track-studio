import api from '../services/api';
import { readFileSync } from 'node:fs';
import { parseSlicerGcode, readSlicerFiles, MAX_SLICER_BYTES } from './slicerImport';
vi.mock('../services/api', () => ({ default: { post: vi.fn() } }));
const fixture = name => readFileSync(`visual/fixtures/${name}`, 'utf8');
const file = (name, text, size = text.length) => ({ name, size, text: async () => text });

it('imports independent plates and mixed tools without multiplying estimates or inventing waste', async () => {
  const plates = await readSlicerFiles(['plate-a.gcode', 'plate-b.gcode'].map(name => file(name, fixture(name))));
  expect(plates.map(p => p.estimated_time_minutes)).toEqual([63, 25]);
  expect(plates[0].materials.map(m => [m.source_tool, m.weight_used])).toEqual([[1, 12.5], [3, 4.25]]);
  expect(plates[0].materials[0]).toMatchObject({ filament_id: '', waste_weight_g: null, measurement_state: 'estimated', estimated_weight_g: 12.5 });
  expect(plates[0].items).toBeUndefined();
});
it('leaves absent duration and grams unknown and ignores length, totals and silent-mode time', () => {
  expect(parseSlicerGcode('; filament used [g] = 1').estimated_time_minutes).toBeNull();
  expect(parseSlicerGcode('; estimated printing time (normal mode) = 0s').materials).toEqual([]);
  expect(() => parseSlicerGcode('; total filament used [g] = 3\n; filament used [mm] = 100\n; estimated printing time (silent mode) = 2h')).toThrow();
});
it.each(['NaN', 'Infinity', '-2', '1e99', '1,,2', '1000000001', '3g'])('rejects malformed grams %s', value => {
  expect(() => parseSlicerGcode(`; filament used [g] = ${value}`)).toThrow();
});
it.each(['-1m', '1m junk', '1m 1h', '999999999999999d', ''])('rejects malformed time %s', value => {
  expect(() => parseSlicerGcode(`; estimated printing time (normal mode) = ${value}`)).toThrow();
});
it('rejects duplicate summaries, binary data, unsupported files and oversized batches', async () => {
  expect(() => parseSlicerGcode('; filament used [g] = 1\n; filament used [g] = 2')).toThrow();
  expect(() => parseSlicerGcode('\0; filament used [g] = 1')).toThrow();
  for (const files of [[file('a.zip', 'data')], [file('a.gcode', '', MAX_SLICER_BYTES + 1)], Array.from({ length: 17 }, () => file('a.gcode', '')), []]) await expect(readSlicerFiles(files)).rejects.toThrow();
});
it('rejects the whole batch when a later plate is invalid or unreadable', async () => {
  await expect(readSlicerFiles([file('a.gcode', fixture('plate-a.gcode')), file('b.gcode', 'invalid')])).rejects.toThrow();
  await expect(readSlicerFiles([{ name: 'a.gcode', size: 1, text: async () => { throw new Error('read failed'); } }])).rejects.toThrow();
});

it('uploads 3MF as multipart and keeps returned plate layout counts', async () => {
  const plates = [{ source_label: '3MF plate 1', items: [{ source_model: 'Synthetic bracket', planned: 1 }], materials: [], estimated_time_minutes: null }];
  api.post.mockResolvedValue({ data: { plates } });
  const result = await readSlicerFiles([new File(['synthetic'], 'layout.3mf')]);
  expect(result).toEqual(plates);
  expect(api.post).toHaveBeenCalledWith('/v2/production/slicer/inspect', expect.any(FormData), { headers: { 'Content-Type': 'multipart/form-data' } });
});
