async function loadTariffData() {
    if (TARIFF) return TARIFF;
    const status = $('tariffStatus');
    if (status) status.textContent = 'Đang nạp biểu thuế...';
    try {
        TARIFF = await fetchTariffDataset();
        if (status) {
            status.textContent = TARIFF.records.length.toLocaleString('vi-VN') + ' dòng HS · dữ liệu ' + TARIFF.as_of;
            status.className = 'tag green';
        }
        return TARIFF;
    } catch (error) {
        if (status) {
            status.textContent = 'Lỗi nạp dữ liệu';
            status.className = 'tag red';
        }
        throw error;
    }
}

function hspoShowHS6MFN(hs) {
    const x = TARIFF?._byhs?.get(String(hs));
    const peers = hspoHS6MFNPeers(x, TARIFF?._hs6Groups);
    if (!x||!peers.length)return;
    const summary = `Mã đang tra: ${fmtHS(x.hs)} · NKƯĐ (MFN) ${hspoMFNValue(x)}%\n`+peers.map(z=>`${fmtHS(z.hs)} · MFN ${hspoMFNValue(z)}%`).join('\n');
    alert('THUẾ SUẤT HS6 — ĐỐI CHIẾU\n\n'+summary+'\n\nChỉ so sánh mã cùng HS6, cùng trường ngày hiệu lực trong bộ dữ liệu. Phải xác định mã HS từ bản chất hàng hóa, không chọn theo mức thuế.');
}

function hspoHS6MFNTag(x) {
    if (!hspoHS6MFNPeers(x, TARIFF?._hs6Groups).length)return '';
    return `<button type="button" class="tag canonicalWarn amber" title="Mã đang tra có thuế NKƯĐ thấp hơn mã HS 8 số khác cùng HS6; bấm xem đối chiếu" onclick="event.stopPropagation();hspoShowHS6MFN('${esc(x.hs)}')">Thuế suất HS6</button>`;
}

function renderTariff() {
    TARIFF_VISIBLE = tariffVisibleRows();
    const rows = TARIFF_VISIBLE.map((x, i) => {
        const child = !!x._pathContext;
        const exp = TARIFF_EXPANDED.has(x.hs);
        const btn = tariffHasChildren(x)?`<button class="hsTreeBtn" title="${exp?'Thu gọn':'Mở mã con'}" onclick="toggleTariffNode('${esc(x.hs)}',event)">${exp?'▾':'▸'}</button>`:'<span style="display:inline-block;width:29px"></span>';
        const role = child?`<div class="pathRole">${x.level===4?'Ông 4 số':x.level===6?'Cha 6 số':'Ngữ cảnh'}</div>`:'';
        const ps = QUALITY_DATA ? policyRowsForHS(x.hs) : [];
        return `<tr class="click ${child?'treeChild pathContextRow':''}" onclick="pickTariff(${i},this)"><td class="hsIndent${x.level}">${btn}<b>${esc(fmtHS(x.hs))}</b>${role}</td><td>${esc(x.vi)}${x._policyHit?'<div class="pathRole">Khớp mô tả chính sách</div>':''}${x._policyOnly?'<div><span class="tag conditional">CHƯA KHỚP TT31</span></div>':''}</td><td>${canonicalWarningHtml({hs:x.hs,vi:x.vi,en:x.en,_p4:x._p4,_p6:x._p6},ps)}${hspoHS6MFNTag(x)}</td><td><span class="tariffPolicyCount">${ps.length}</span></td><td>${esc(rateVal(x,'NKUD'))}</td><td>${esc(rateVal(x,'VAT'))}</td></tr>`
    }).join('');
    $('tariffRows').innerHTML = rows || '<tr><td colspan="6" class="empty">Không có kết quả</td></tr>';
    setTimeout(()=>enableColumnResize('tariffTable'),20)
}
