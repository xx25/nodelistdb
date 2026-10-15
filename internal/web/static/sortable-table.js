/**
 * Sortable Table JavaScript
 * Makes tables sortable by clicking on column headers
 */

(function() {
    'use strict';

    // Initialize sortable tables on page load
    document.addEventListener('DOMContentLoaded', function() {
        const tables = document.querySelectorAll('table.sortable-table');
        tables.forEach(initSortableTable);
    });

    function initSortableTable(table) {
        const headers = table.querySelectorAll('thead th[data-sortable]');
        const tbody = table.querySelector('tbody');

        if (!tbody) return;

        let defaultColumnHeader = null;
        let defaultOrder = 'desc'; // Default to descending

        headers.forEach(header => {
            // The header's appearance (cursor, indicator placement) lives in
            // style.css under .sortable-table th[data-sortable]; nothing is set
            // inline, so the stylesheet can still make the first column sticky
            // on phones - an inline position:relative would win over that.
            header.setAttribute('tabindex', '0');
            header.setAttribute('role', 'columnheader');
            header.setAttribute('aria-sort', 'none');

            // Add sort indicator
            const indicator = document.createElement('span');
            indicator.className = 'sort-indicator';
            indicator.setAttribute('aria-hidden', 'true');
            indicator.textContent = '⇅';
            header.appendChild(indicator);

            // Check if this column has data-default-sort attribute
            if (header.hasAttribute('data-default-sort')) {
                defaultColumnHeader = header;
                defaultOrder = header.getAttribute('data-default-sort') || 'desc';
            }
            // Otherwise check if this is a date column (default sort behavior)
            else {
                const dataType = header.getAttribute('data-type');
                if (dataType === 'date' && defaultColumnHeader === null) {
                    // Remember this column for initial sorting (only if no explicit default set)
                    defaultColumnHeader = header;
                    defaultOrder = 'desc'; // Newest first for dates
                }
            }

            // The column to sort is the header's own position in its row
            // (cellIndex), not its position among the sortable headers: a
            // non-sortable column before this one used to shift every sort
            // onto the wrong cells, on first paint as well as on click.
            const sort = () => sortTable(table, tbody, header.cellIndex, header);
            header.addEventListener('click', sort);
            header.addEventListener('keydown', function(e) {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    sort();
                }
            });
        });

        // Perform initial sort on default column
        if (defaultColumnHeader) {
            // Set opposite order first so sortTable will toggle to desired order
            defaultColumnHeader.setAttribute('data-order', defaultOrder === 'desc' ? 'asc' : 'desc');
            sortTable(table, tbody, defaultColumnHeader.cellIndex, defaultColumnHeader);
        }
    }

    function sortTable(table, tbody, columnIndex, header) {
        const rows = Array.from(tbody.querySelectorAll('tr'));
        const dataType = header.getAttribute('data-type') || 'string';
        const currentOrder = header.getAttribute('data-order') || 'none';
        const newOrder = currentOrder === 'asc' ? 'desc' : 'asc';

        // Clear all sort indicators
        table.querySelectorAll('thead th').forEach(th => {
            th.setAttribute('data-order', 'none');
            if (th.hasAttribute('aria-sort')) th.setAttribute('aria-sort', 'none');
            const indicator = th.querySelector('.sort-indicator');
            if (indicator) indicator.textContent = '⇅';
        });

        // Update current column indicator
        header.setAttribute('data-order', newOrder);
        header.setAttribute('aria-sort', newOrder === 'asc' ? 'ascending' : 'descending');
        const indicator = header.querySelector('.sort-indicator');
        if (indicator) indicator.textContent = newOrder === 'asc' ? '▲' : '▼';

        // Sort rows
        rows.sort((rowA, rowB) => {
            const cellA = rowA.cells[columnIndex];
            const cellB = rowB.cells[columnIndex];

            let valueA = getCellValue(cellA, dataType);
            let valueB = getCellValue(cellB, dataType);

            let comparison = 0;

            if (dataType === 'number') {
                comparison = (valueA || 0) - (valueB || 0);
            } else if (dataType === 'date') {
                // If data-value contains Unix timestamp (number), parse as number
                // Otherwise try to parse as date string
                const numA = parseFloat(valueA);
                const numB = parseFloat(valueB);
                if (!isNaN(numA) && !isNaN(numB)) {
                    comparison = numA - numB;
                } else {
                    const dateA = valueA ? new Date(valueA) : new Date(0);
                    const dateB = valueB ? new Date(valueB) : new Date(0);
                    comparison = dateA - dateB;
                }
            } else if (dataType === 'address') {
                // Special sorting for FidoNet addresses (zone:net/node)
                const addrA = parseAddress(valueA);
                const addrB = parseAddress(valueB);
                comparison = compareAddresses(addrA, addrB);
            } else {
                // String comparison
                valueA = valueA || '';
                valueB = valueB || '';
                comparison = valueA.localeCompare(valueB, undefined, { numeric: true, sensitivity: 'base' });
            }

            return newOrder === 'asc' ? comparison : -comparison;
        });

        // Re-append rows in sorted order
        rows.forEach(row => tbody.appendChild(row));
    }

    function getCellValue(cell, dataType) {
        // An empty-state row with a single colspan cell has no cell at this
        // index; sort it as empty rather than throwing.
        if (!cell) return '';

        // Try data attribute first
        if (cell.hasAttribute('data-value')) {
            return cell.getAttribute('data-value');
        }

        // Extract text content
        let text = cell.textContent.trim();

        // For links, get the text inside the link
        const link = cell.querySelector('a');
        if (link) {
            text = link.textContent.trim();
        }

        // For badges, get the first badge text
        const badge = cell.querySelector('.badge');
        if (badge && dataType === 'string') {
            text = badge.textContent.trim();
        }

        return text;
    }

    function parseAddress(addressStr) {
        // Parse FidoNet address format: zone:net/node
        const match = (addressStr || '').match(/(\d+):(\d+)\/(\d+)/);
        if (match) {
            return {
                zone: parseInt(match[1], 10),
                net: parseInt(match[2], 10),
                node: parseInt(match[3], 10)
            };
        }
        return { zone: 0, net: 0, node: 0 };
    }

    function compareAddresses(a, b) {
        if (a.zone !== b.zone) return a.zone - b.zone;
        if (a.net !== b.net) return a.net - b.net;
        return a.node - b.node;
    }

})();
