/**
 * Unified Tooltip System with Copy Functionality
 * Handles all tooltip types across the application:
 * - IP address tooltips (.ip-tooltip with data-ips)
 * - IPv6 address tooltips (.ipv6-tooltip with data-ipv6)
 * - Flag tooltips (.flag-tooltip with nested .tooltip-text)
 * - Any element with data-copyable attribute
 */

(function() {
    'use strict';

    /**
     * Initialize tooltips on page load
     */
    function initTooltips() {
        // Find all elements with copyable data
        const tooltipElements = document.querySelectorAll(
            '[data-ips], [data-ipv6], [data-copyable], .ip-tooltip, .ipv6-tooltip'
        );

        tooltipElements.forEach(function(element) {
            addCopyButtons(element);
        });
    }

    /**
     * Parse IP data to separate IPv4 and IPv6 addresses
     * @param {string} ipData - Raw IP data string
     * @returns {Object} Object with ipv4 and ipv6 arrays
     */
    function parseIPData(ipData) {
        if (!ipData) return { ipv4: [], ipv6: [] };

        const lines = ipData.split('\n');
        const ipv4 = [];
        const ipv6 = [];
        let currentSection = null;

        lines.forEach(function(line) {
            line = line.trim();
            if (line === 'IPv4:') {
                currentSection = 'ipv4';
            } else if (line === 'IPv6:') {
                currentSection = 'ipv6';
            } else if (line && currentSection) {
                if (currentSection === 'ipv4') {
                    ipv4.push(line);
                } else if (currentSection === 'ipv6') {
                    ipv6.push(line);
                }
            } else if (line && !currentSection) {
                // If no section header, guess based on IP format
                if (line.includes(':') && (line.match(/:/g) || []).length > 1) {
                    ipv6.push(line);
                } else if (line.match(/^\d+\.\d+\.\d+\.\d+$/)) {
                    ipv4.push(line);
                }
            }
        });

        return { ipv4: ipv4, ipv6: ipv6 };
    }

    /**
     * Add copy buttons to a tooltip element
     * @param {HTMLElement} element - The element to add copy buttons to
     */
    function addCopyButtons(element) {
        // Check if already processed
        if (element.querySelector('.tooltip-copy-group') || element.hasAttribute('data-tooltip-processed')) {
            return;
        }

        // Get the copyable content from various data attributes
        const ipData = element.getAttribute('data-ips') || element.getAttribute('data-ipv6');

        // Get hostname from element text content
        const hostname = element.textContent.trim();
        // Remove any existing suffixes like "(primary)" or "(backup #N)"
        const cleanHostname = hostname.replace(/\s*\(.*?\)\s*/g, '').trim();

        // Parse IP data (if available)
        const parsed = ipData ? parseIPData(ipData) : { ipv4: [], ipv6: [] };

        // Create container for copy buttons (only if there's IP data to copy)
        let copyGroup = null;
        if (ipData && (parsed.ipv4.length > 0 || parsed.ipv6.length > 0 || cleanHostname)) {
            copyGroup = document.createElement('span');
            copyGroup.className = 'tooltip-copy-group';

            // Create copy hostname button
            const copyHostnameBtn = createCopyButton('🏷️', 'Copy hostname', function() {
                copyToClipboard(cleanHostname, copyHostnameBtn);
            });
            copyGroup.appendChild(copyHostnameBtn);

            // Create copy IPv4 button (if IPv4 addresses exist)
            if (parsed.ipv4.length > 0) {
                const ipv4Text = parsed.ipv4.join('\n');
                const copyIPv4Btn = createCopyButton('4️⃣', 'Copy IPv4 addresses', function() {
                    copyToClipboard(ipv4Text, copyIPv4Btn);
                });
                copyGroup.appendChild(copyIPv4Btn);
            }

            // Create copy IPv6 button (if IPv6 addresses exist)
            if (parsed.ipv6.length > 0) {
                const ipv6Text = parsed.ipv6.join('\n');
                const copyIPv6Btn = createCopyButton('6️⃣', 'Copy IPv6 addresses', function() {
                    copyToClipboard(ipv6Text, copyIPv6Btn);
                });
                copyGroup.appendChild(copyIPv6Btn);
            }

            // Create copy all button
            const copyAllBtn = createCopyButton('📋', 'Copy all (hostname + all IPs)', function() {
                let combined = 'Hostname: ' + cleanHostname;
                if (parsed.ipv4.length > 0) {
                    combined += '\n\nIPv4:\n' + parsed.ipv4.join('\n');
                }
                if (parsed.ipv6.length > 0) {
                    combined += '\n\nIPv6:\n' + parsed.ipv6.join('\n');
                }
                copyToClipboard(combined, copyAllBtn);
            });
            copyGroup.appendChild(copyAllBtn);
        }

        // Special handling for badge elements to prevent background extending over buttons
        if (element.classList.contains('badge')) {
            // Save original content (hostname text)
            const originalContent = element.innerHTML;

            // Wrap the original content in a span to isolate the badge background
            const contentWrapper = document.createElement('span');
            contentWrapper.innerHTML = originalContent;

            // Copy badge classes to the wrapper (but not tooltip classes)
            const badgeClasses = Array.from(element.classList).filter(function(cls) {
                return cls === 'badge' || cls.startsWith('badge-') || cls === 'text-muted';
            });
            contentWrapper.className = badgeClasses.join(' ');

            // Ensure the wrapper displays as inline-block to properly show badge styling
            contentWrapper.style.display = 'inline-block';

            // Clear element and rebuild structure
            element.innerHTML = '';
            element.appendChild(contentWrapper);

            // Add copy buttons if they exist
            if (copyGroup) {
                element.appendChild(copyGroup);
            }

            // Style the parent element as flex container
            element.style.display = 'inline-flex';
            element.style.alignItems = 'center';
            element.style.gap = '0.35rem';
            element.style.flexWrap = 'wrap';
            element.style.background = 'none'; // Remove badge background from parent
            element.style.padding = '0'; // Remove padding from parent
            element.style.border = 'none'; // Remove border from parent

            // Mark as processed
            element.setAttribute('data-tooltip-processed', 'true');
        } else if (copyGroup) {
            // Standard handling for non-badge elements (only if there are copy buttons)
            const computedDisplay = window.getComputedStyle(element).display;
            if (computedDisplay === 'inline' || computedDisplay === 'inline-block') {
                element.style.display = 'inline-flex';
                element.style.alignItems = 'center';
                element.style.gap = '0.35rem';
                element.style.flexWrap = 'wrap';
            }

            // Append copy group to element
            element.appendChild(copyGroup);

            // Mark as processed
            element.setAttribute('data-tooltip-processed', 'true');
        }
    }

    /**
     * Create a copy button
     * @param {string} icon - Button icon/emoji
     * @param {string} title - Button title/tooltip
     * @param {Function} onClick - Click handler
     * @returns {HTMLElement} Button element
     */
    function createCopyButton(icon, title, onClick) {
        const btn = document.createElement('button');
        btn.className = 'tooltip-copy-btn';
        btn.innerHTML = icon;
        btn.setAttribute('type', 'button');
        btn.setAttribute('title', title);
        btn.setAttribute('aria-label', title);

        // Add click handler
        btn.addEventListener('click', function(e) {
            e.preventDefault();
            e.stopPropagation();
            onClick();
        });

        // Add keyboard handler for accessibility
        btn.addEventListener('keydown', function(e) {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                e.stopPropagation();
                onClick();
            }
        });

        return btn;
    }

    /**
     * Copy text to clipboard with visual feedback
     * @param {string} text - Text to copy
     * @param {HTMLElement} button - Button element for visual feedback
     */
    function copyToClipboard(text, button) {
        const originalHTML = button.innerHTML;
        const originalColor = button.style.color;

        // Try modern clipboard API first
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text)
                .then(function() {
                    showCopySuccess(button, originalHTML, originalColor);
                })
                .catch(function(err) {
                    console.warn('Clipboard API failed, trying fallback:', err);
                    fallbackCopy(text, button, originalHTML, originalColor);
                });
        } else {
            // Fallback for older browsers
            fallbackCopy(text, button, originalHTML, originalColor);
        }
    }

    /**
     * Fallback copy method for older browsers
     * @param {string} text - Text to copy
     * @param {HTMLElement} button - Button element
     * @param {string} originalHTML - Original button HTML
     * @param {string} originalColor - Original button color
     */
    function fallbackCopy(text, button, originalHTML, originalColor) {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.left = '-9999px';
        textarea.style.top = '0';
        textarea.style.opacity = '0';
        textarea.setAttribute('readonly', '');

        document.body.appendChild(textarea);

        try {
            textarea.select();
            textarea.setSelectionRange(0, text.length); // For mobile devices

            const successful = document.execCommand('copy');
            if (successful) {
                showCopySuccess(button, originalHTML, originalColor);
            } else {
                showCopyError(button, originalHTML, originalColor);
            }
        } catch (err) {
            console.error('Fallback copy failed:', err);
            showCopyError(button, originalHTML, originalColor);
        }

        document.body.removeChild(textarea);
    }

    /**
     * Show success feedback on copy button
     * @param {HTMLElement} button - Button element
     * @param {string} originalHTML - Original button HTML
     * @param {string} originalColor - Original button color
     */
    function showCopySuccess(button, originalHTML, originalColor) {
        button.innerHTML = '✓';
        button.style.color = '#10b981'; // success green
        const originalTitle = button.getAttribute('title');
        button.setAttribute('title', 'Copied!');

        setTimeout(function() {
            button.innerHTML = originalHTML;
            button.style.color = originalColor;
            button.setAttribute('title', originalTitle);
        }, 1500);
    }

    /**
     * Show error feedback on copy button
     * @param {HTMLElement} button - Button element
     * @param {string} originalHTML - Original button HTML
     * @param {string} originalColor - Original button color
     */
    function showCopyError(button, originalHTML, originalColor) {
        button.innerHTML = '✗';
        button.style.color = '#ef4444'; // error red
        const originalTitle = button.getAttribute('title');
        button.setAttribute('title', 'Copy failed');

        setTimeout(function() {
            button.innerHTML = originalHTML;
            button.style.color = originalColor;
            button.setAttribute('title', originalTitle);
        }, 1500);
    }

    /**
     * Initialize on DOM ready
     */
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initTooltips);
    } else {
        // DOM already loaded
        initTooltips();
    }

    /**
     * Expose public API for dynamic content
     * Usage: window.TooltipSystem.refresh()
     */
    window.TooltipSystem = {
        refresh: initTooltips,
        addCopyButtons: addCopyButtons
    };

})();
