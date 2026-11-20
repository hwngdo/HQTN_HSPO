function hspoMFNValue(record) {
    const rate = record?.rates?.NKUD;
    if (!Array.isArray(rate)) return null;
    const text = String(rate[0] ?? '').trim().replace(',', '.');
    return /^\d+(?:\.\d+)?$/.test(text) ? Number(text) : null;
}

function hspoHS6MFNPeers(record, groups) {
    const hs = String(record?.hs || '').replace(/\D/g, '');
    if (hs.length !== 8 || !groups) return [];

    const rate = hspoMFNValue(record);
    if (rate === null) return [];
    const effectiveDate = String(record?.rates?.NKUD?.[2] || '');
    return (groups.get(hs.slice(0, 6)) || [])
        .filter(peer => {
            const peerRate = hspoMFNValue(peer);
            return String(peer.hs) !== hs && peerRate !== null && peerRate > rate &&
                String(peer.rates.NKUD[2] || '') === effectiveDate;
        })
        .sort((first, second) => hspoMFNValue(first) - hspoMFNValue(second));
}
