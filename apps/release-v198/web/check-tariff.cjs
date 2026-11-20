const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { gunzipSync } = require('node:zlib');

const tariffFile = process.argv[2];
assert.ok(tariffFile, 'Pass the imported tariff_2026.json.gz path');
const dataset = JSON.parse(gunzipSync(fs.readFileSync(tariffFile)));
const groups = new Map();
for (const row of dataset.records) {
    if (row.hs.length !== 8) continue;
    const hs6 = row.hs.slice(0, 6);
    if (!groups.has(hs6)) groups.set(hs6, []);
    groups.get(hs6).push(row);
}

const web = path.join(__dirname, 'static');
const html = fs.readFileSync(path.join(web, 'index.html'), 'utf8');
const scripts = Array.from(html.matchAll(/<script src="\/static\/(tariff-[^"]+)"><\/script>/g), m => m[1]);
assert.deepEqual(scripts, ['tariff-data.js', 'tariff-warnings.js', 'tariff-view.js']);
for (const match of html.matchAll(/<script>([\s\S]*?)<\/script>/g)) new vm.Script(match[1]);

const messages = [];
const ctx = vm.createContext({
    TARIFF: { _hs6Groups: groups, _byhs: new Map(dataset.records.map(row => [row.hs, row])) },
    esc: String, fmtHS: String, alert: message => messages.push(message),
});
for (const script of scripts) vm.runInContext(fs.readFileSync(path.join(web, script), 'utf8'), ctx);
const low = ctx.TARIFF._byhs.get('01013010');
const high = ctx.TARIFF._byhs.get('01013090');
assert.equal(ctx.hspoMFNValue(low), 0);
assert.equal(ctx.hspoMFNValue(high), 5);
assert.ok(ctx.hspoHS6MFNPeers(low, groups).includes(high));
assert.ok(!ctx.hspoHS6MFNPeers(high, groups).includes(low));
assert.match(ctx.hspoHS6MFNTag(low), /Thuế suất HS6/);
ctx.hspoShowHS6MFN(low.hs);
assert.match(messages[0], /01013090 · MFN 5%/);
console.log('HS6 checks passed against the imported v1.9.8 tariff.');
