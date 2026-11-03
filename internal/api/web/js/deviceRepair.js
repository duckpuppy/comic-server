// Re-pair a device (tracker #53): when an app makes a new Sync ID (reinstall,
// cleared data, new phone), move the old device's lists, settings and history
// to the new ID. The server does the move in one transaction.
const deviceRepair = {
    // Same rule as the server: letters, digits and . _ : - only.
    ID_RE: /^[A-Za-z0-9._:-]{1,128}$/,

    // Pick the new device for oldDevice ({id, name}), confirm, and move.
    // Resolves to the new device ID when it moved, otherwise null.
    async start(oldDevice) {
        const candidates = await this.loadCandidates(oldDevice);
        const picked = await this.pickNewDevice(oldDevice, candidates);
        if (!picked) return null;
        const moved = await this.confirmAndRepair(oldDevice, picked.id, picked.label);
        return moved ? picked.id : null;
    },

    // Devices the server has seen but that are not registered, same name first.
    async loadCandidates(oldDevice) {
        try {
            const response = await fetch('/api/devices');
            const data = await response.json();
            const wanted = (oldDevice.name || '').trim().toLowerCase();
            const nameOf = d => (d.friendly_name || d.name || '').trim().toLowerCase();
            return (data.devices || [])
                .filter(d => !d.is_registered && d.id !== oldDevice.id)
                .sort((a, b) => (nameOf(b) === wanted) - (nameOf(a) === wanted) || nameOf(a).localeCompare(nameOf(b)));
        } catch (error) {
            console.error('Failed to load devices for re-pair:', error);
            return [];
        }
    },

    pickNewDevice(oldDevice, candidates) {
        return new Promise(resolve => {
            const esc = text => dialogs.escapeHtml(text == null ? '' : String(text));
            const wanted = (oldDevice.name || '').trim().toLowerCase();
            const rows = candidates.map((d, i) => {
                const label = d.friendly_name || d.name || d.id;
                const same = label.trim().toLowerCase() === wanted;
                return `
                    <label class="repair-option">
                        <input type="radio" name="repair-new" value="${esc(d.id)}" data-label="${esc(label)}" ${i === 0 ? 'checked' : ''}>
                        <span>
                            <strong>${esc(label)}</strong>${same ? ' <em>(same name)</em>' : ''}<br>
                            <small>${esc(d.manufacturer)} ${esc(d.model)} &middot; IP ${esc(d.ip)} &middot; ID ${esc(d.id)}</small>
                        </span>
                    </label>`;
            }).join('');

            const bodyHTML = `
                <p>Move the lists, settings and history of <strong>${esc(oldDevice.name)}</strong> to a new device ID.</p>
                ${rows || '<p class="empty-message">No unregistered device has been seen. Open the app on the new device and wait a moment, or type its ID below.</p>'}
                <label class="repair-manual">Or type a device ID
                    <input type="text" id="repair-manual-id" class="form-control" autocomplete="off" spellcheck="false" placeholder="Pairing ID of the new device">
                </label>
                <p class="repair-error" id="repair-error" style="display:none"></p>`;

            let picked = null;
            const close = dialogs.custom({
                title: 'Re-pair device',
                bodyHTML,
                footerHTML: `
                    <button class="btn btn-secondary" data-action="cancel">Cancel</button>
                    <button class="btn btn-primary" id="repair-continue">Continue</button>`,
                onOpen: () => {
                    const modal = document.getElementById('dialogs-modal');
                    modal.querySelector('#repair-continue').addEventListener('click', () => {
                        const typed = modal.querySelector('#repair-manual-id').value.trim();
                        const radio = modal.querySelector('input[name="repair-new"]:checked');
                        const id = typed || (radio ? radio.value : '');
                        const errorEl = modal.querySelector('#repair-error');
                        if (!id || !this.ID_RE.test(id)) {
                            errorEl.textContent = 'Pick a device or type a valid device ID (letters, digits and . _ : - only).';
                            errorEl.style.display = 'block';
                            return;
                        }
                        picked = { id, label: typed ? typed : radio.dataset.label };
                        close();
                    });
                },
                onClose: () => resolve(picked),
            });
        });
    },

    // Confirm with old and new ID, then ask the server to move. Resolves true
    // when it moved. A failure shows the server's reason and nothing changes.
    async confirmAndRepair(oldDevice, newId, newLabel) {
        const ok = await dialogs.confirm({
            title: 'Re-pair device',
            message: `Move "${oldDevice.name}" (ID ${oldDevice.id}) to ${newLabel && newLabel !== newId ? `"${newLabel}" (ID ${newId})` : `ID ${newId}`}? ` +
                'Its lists, settings and history move to the new ID, and the old ID stops syncing. ' +
                'To undo it, re-pair from the new ID back to the old one.',
            confirmLabel: 'Re-pair',
        });
        if (!ok) return false;

        try {
            const response = await fetch(`/api/devices/${encodeURIComponent(oldDevice.id)}/repair`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ new_device_id: newId }),
            });
            if (!response.ok) {
                const text = await response.text();
                dialogs.toast(text || 'Re-pair failed.', 'error');
                return false;
            }
            dialogs.toast(`Re-paired. "${oldDevice.name}" now uses ID ${newId}.`, 'success');
            return true;
        } catch (error) {
            console.error('Re-pair failed:', error);
            dialogs.toast('Re-pair failed. Nothing was changed.', 'error');
            return false;
        }
    },
};
