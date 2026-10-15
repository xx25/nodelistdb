/**
 * Horizontal-scroll affordance for wide tables.
 *
 * Every .table-responsive / .table-container is wrapped in a .table-scroll
 * element that carries data-more-left / data-more-right while the inner box
 * can scroll further that way; style.css draws edge shadows from those
 * attributes (phones only) and shows a one-line hint until the reader has
 * scrolled once. Every wrapper is created at load, whether or not it
 * overflows right now: a table inside a closed <details> measures 0 wide
 * until it is opened, and a table that fits at desktop width overflows
 * after a rotation, so the state is re-measured by a ResizeObserver rather
 * than decided once.
 */
(function () {
    'use strict';

    function update(outer, inner) {
        var max = inner.scrollWidth - inner.clientWidth;
        if (max > 1 && inner.scrollLeft > 1) outer.setAttribute('data-more-left', '');
        else outer.removeAttribute('data-more-left');
        if (max > 1 && inner.scrollLeft < max - 1) outer.setAttribute('data-more-right', '');
        else outer.removeAttribute('data-more-right');
        var hint = outer.previousElementSibling;
        if (hint && hint.classList.contains('table-scroll__hint')) {
            if (max > 1 && !outer.hasAttribute('data-scrolled')) hint.setAttribute('data-show', '');
            else hint.removeAttribute('data-show');
        }
    }

    function wrap(inner) {
        if (inner.parentElement && inner.parentElement.classList.contains('table-scroll')) return;
        var outer = document.createElement('div');
        outer.className = 'table-scroll' + (inner.classList.contains('table-container') ? ' table-scroll--container' : '');
        inner.parentNode.insertBefore(outer, inner);
        outer.appendChild(inner);

        // The hint goes BEFORE the wrapper: a card's last child loses its
        // bottom margin (.card > :last-child), and that must stay the table.
        var hint = document.createElement('p');
        hint.className = 'table-scroll__hint';
        hint.setAttribute('aria-hidden', 'true');
        hint.textContent = 'Swipe sideways to see more columns →';
        outer.parentNode.insertBefore(hint, outer);

        inner.addEventListener('scroll', function () {
            if (inner.scrollLeft > 1) outer.setAttribute('data-scrolled', '');
            update(outer, inner);
        }, { passive: true });
        if (typeof ResizeObserver === 'function') {
            new ResizeObserver(function () { update(outer, inner); }).observe(inner);
        } else {
            window.addEventListener('resize', function () { update(outer, inner); });
        }
        update(outer, inner);
    }

    function init() {
        document.querySelectorAll('.table-responsive, .table-container').forEach(wrap);
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
})();
