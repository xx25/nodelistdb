// Global FTN network switcher (nav bar).
//
// The selected network is stored in the ftn_network cookie, which the server
// reads as the default for every page (an explicit ?domain= URL parameter
// still wins). The list of networks comes from /api/networks and is cached in
// sessionStorage so navigation does not hit the API on every page view. The
// switcher stays hidden when only one network is imported.
(function () {
    'use strict';

    var COOKIE = 'ftn_network';
    var DEFAULT_NETWORK = 'fidonet';
    var CACHE_KEY = 'ndb_networks_v1';
    var CACHE_TTL_MS = 60 * 60 * 1000; // 1 hour
    var NAME_RE = /^[a-z0-9_-]{1,32}$/;

    function getCookie(name) {
        var parts = document.cookie.split(';');
        for (var i = 0; i < parts.length; i++) {
            var kv = parts[i].trim();
            if (kv.indexOf(name + '=') === 0) {
                return decodeURIComponent(kv.substring(name.length + 1));
            }
        }
        return null;
    }

    function setCookie(name, value) {
        document.cookie = name + '=' + encodeURIComponent(value) +
            '; path=/; max-age=' + (365 * 24 * 60 * 60) + '; samesite=lax';
    }

    function currentNetwork() {
        var v = getCookie(COOKIE);
        return v && NAME_RE.test(v) ? v : DEFAULT_NETWORK;
    }

    function cachedNetworks() {
        try {
            var raw = sessionStorage.getItem(CACHE_KEY);
            if (!raw) return null;
            var entry = JSON.parse(raw);
            if (!entry || !Array.isArray(entry.networks)) return null;
            if (Date.now() - entry.at > CACHE_TTL_MS) return null;
            return entry.networks;
        } catch (e) {
            return null;
        }
    }

    function storeNetworks(networks) {
        try {
            sessionStorage.setItem(CACHE_KEY, JSON.stringify({ at: Date.now(), networks: networks }));
        } catch (e) { /* storage full/disabled: just refetch next time */ }
    }

    function fetchNetworks() {
        var cached = cachedNetworks();
        if (cached) return Promise.resolve(cached);
        return fetch('/api/networks', { headers: { 'Accept': 'application/json' } })
            .then(function (resp) {
                if (!resp.ok) throw new Error('http ' + resp.status);
                return resp.json();
            })
            .then(function (data) {
                var list = (data && data.networks ? data.networks : [])
                    .map(function (n) { return n.domain; })
                    .filter(function (d) { return typeof d === 'string' && NAME_RE.test(d); });
                storeNetworks(list);
                return list;
            });
    }

    // On the domain-expiration analytics pages ?domain= is a DNS hostname, not
    // an FTN network, and the registrars page is not network-scoped at all —
    // leave the URL alone there.
    function isDNSDomainPage() {
        return window.location.pathname.indexOf('/analytics/domain-expiration') === 0 ||
            window.location.pathname.indexOf('/analytics/registrars') === 0;
    }

    function switchTo(network) {
        setCookie(COOKIE, network);
        var url = new URL(window.location.href);
        if (!isDNSDomainPage()) {
            // Keep the landed-on URL shareable: encode non-default networks
            // explicitly (the cookie alone wouldn't travel with a bookmark).
            if (network === DEFAULT_NETWORK) {
                url.searchParams.delete('domain');
            } else {
                url.searchParams.set('domain', network);
            }
        }
        window.location.href = url.toString();
    }

    function init() {
        var wrap = document.getElementById('network-switch');
        var select = document.getElementById('network-switch-select');
        if (!wrap || !select) return;

        fetchNetworks().then(function (networks) {
            if (networks.length < 2) {
                // Single-network install: switcher stays hidden — but heal a
                // stale cookie pointing at a network that no longer exists,
                // which would otherwise blank every page with no visible fix.
                if (networks.length === 1 && currentNetwork() !== networks[0]) {
                    setCookie(COOKIE, networks[0]);
                    // Reload only if the write took — avoids a reload loop
                    // when cookies are disabled.
                    if (currentNetwork() === networks[0]) window.location.reload();
                }
                return;
            }

            var current = currentNetwork();
            // An explicit ?domain= in the URL overrides the cookie server-side;
            // reflect what the page actually shows (FTN pages only).
            var explicit = null;
            if (!isDNSDomainPage()) {
                explicit = new URL(window.location.href).searchParams.get('domain');
            }
            if (explicit && NAME_RE.test(explicit)) current = explicit;

            select.innerHTML = '';
            networks.forEach(function (network) {
                var opt = document.createElement('option');
                opt.value = network;
                opt.textContent = network;
                select.appendChild(opt);
            });
            if (networks.indexOf(current) === -1) {
                var extra = document.createElement('option');
                extra.value = current;
                extra.textContent = current;
                select.appendChild(extra);
            }
            select.value = current;
            select.addEventListener('change', function () {
                switchTo(select.value);
            });
            wrap.hidden = false;
        }).catch(function () { /* API unavailable: leave the switcher hidden */ });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
