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

        headers.forEach((header, index) => {
            header.style.cursor = 'pointer';
            header.style.userSelect = 'none';
            header.style.position = 'relative';
            header.style.paddingRight = '24px';

            // Add sort indicator
            const indicator = document.createElement('span');
            indicator.className = 'sort-indicator';
            indicator.innerHTML = '⇅';
            indicator.style.position = 'absolute';
            indicator.style.right = '8px';
            indicator.style.opacity = '0.3';
            indicator.style.fontSize = '14px';
            header.appendChild(indicator);

            // Check if this is the default sorted column (date type = Last Tested)
            const dataType = header.getAttribute('data-type');
            if (dataType === 'date') {
                // Set initial sort state to descending (newest first)
                header.setAttribute('data-order', 'desc');
                indicator.innerHTML = '▼';
                indicator.style.opacity = '1';
            }

            // Add click handler
            header.addEventListener('click', function() {
                sortTable(table, tbody, index, header);
            });
        });
    }

    function sortTable(table, tbody, columnIndex, header) {
        const rows = Array.from(tbody.querySelectorAll('tr'));
        const dataType = header.getAttribute('data-type') || 'string';
        const currentOrder = header.getAttribute('data-order') || 'none';
        const newOrder = currentOrder === 'asc' ? 'desc' : 'asc';

        // Clear all sort indicators
        table.querySelectorAll('thead th').forEach(th => {
            th.setAttribute('data-order', 'none');
            const indicator = th.querySelector('.sort-indicator');
            if (indicator) {
                indicator.innerHTML = '⇅';
                indicator.style.opacity = '0.3';
            }
        });

        // Update current column indicator
        header.setAttribute('data-order', newOrder);
        const indicator = header.querySelector('.sort-indicator');
        if (indicator) {
            indicator.innerHTML = newOrder === 'asc' ? '▲' : '▼';
            indicator.style.opacity = '1';
        }

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
        const match = addressStr.match(/(\d+):(\d+)\/(\d+)/);
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
