import { build } from 'esbuild';
import { readFile, writeFile } from 'node:fs/promises';
import { gzipSync } from 'node:zlib';
await build({
  entryPoints: ['src/app.jsx'],
  bundle: true,
  minify: true,
  outfile: 'static/app.js',
  jsx: 'automatic',
  target: ['es2020'],
  define: { 'process.env.NODE_ENV': '"production"' },
  legalComments: 'linked',
  logLevel: 'info',
});
await writeFile('static/index.html', await readFile('index.html'));
for (const file of ['app.js', 'app.css']) {
  const compressed = gzipSync(await readFile('static/' + file), { level: 9 });
  await writeFile('static/' + file + '.gz', compressed);
  console.log(`${file}: ${(compressed.length / 1024).toFixed(1)} KB gzip`);
}
