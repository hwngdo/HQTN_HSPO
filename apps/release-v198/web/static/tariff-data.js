async function fetchTariffDataset() {
    const dataset = await loadStaticGzipJson(
        versionedStatic('/static/tariff_2026.json.gz'),
        'dữ liệu biểu thuế'
    );
    const vatDefault = dataset.vat_default_rule;
    if (vatDefault) {
        for (const record of dataset.records) {
            const hs = String(record.hs || '');
            if (hs.length !== 8 || hs.startsWith(vatDefault.except_chapter) ||
                !record.rates || record.rates.VAT) continue;
            record.rates.VAT = [vatDefault.rate, vatDefault.reference, vatDefault.valid_from];
            record.vat_detail = vatDefault.rate_detail;
        }
    }

    dataset._byhs = new Map();
    dataset._hs6Groups = new Map();
    dataset.records.forEach((record, index) => {
        record._seq = index;
        record._nvi = normText(record.vi);
        record._nen = normText(record.en);
        record._avi = normKeepAccent(record.vi);
        record._aen = normKeepAccent(record.en);
        record._ownsearch = (record._nvi + ' ' + record._nen).trim();
        record._owntokens = tariffTokens(record._ownsearch);
        record._ownaccent = (record._avi + ' ' + record._aen).trim();
        record._ownaccenttokens = tariffAccentTokens(record._ownaccent);
        record._matchScore = 0;
        if (!dataset._byhs.has(record.hs)) dataset._byhs.set(record.hs, record);

        const hs = String(record.hs || '');
        if (hs.length !== 8) return;
        const group = hs.slice(0, 6);
        if (!dataset._hs6Groups.has(group)) dataset._hs6Groups.set(group, []);
        dataset._hs6Groups.get(group).push(record);
    });

    for (const record of dataset.records) {
        const hs = String(record.hs || '');
        record._p4 = hs.length > 4 ? dataset._byhs.get(hs.slice(0, 4)) || null : null;
        record._p6 = hs.length > 6 ? dataset._byhs.get(hs.slice(0, 6)) || null : null;
    }
    return dataset;
}
