// Simple JavaScript for form enhancements
document.addEventListener('DOMContentLoaded', function() {
    // Auto-focus first input on search page
    const firstInput = document.querySelector('input[type="number"], input[type="text"]');
    if (firstInput) {
        firstInput.focus();
    }
    
    // Handle full address input
    const fullAddressInput = document.getElementById('full_address');
    const zoneInput = document.getElementById('zone');
    const netInput = document.getElementById('net');
    const nodeInput = document.getElementById('node');
    
    if (fullAddressInput) {
        fullAddressInput.addEventListener('input', function() {
            const address = this.value.trim();
            // If full address is being typed, clear individual fields
            if (address && zoneInput && netInput && nodeInput) {
                zoneInput.value = '';
                netInput.value = '';
                nodeInput.value = '';
            }
        });
    }
    
    // Clear full address when individual fields are used
    if (zoneInput || netInput || nodeInput) {
        // Handle zone select dropdown
        if (zoneInput) {
            zoneInput.addEventListener('change', function() {
                if (this.value && fullAddressInput) {
                    fullAddressInput.value = '';
                }
            });
        }
        
        // Handle net and node input fields
        [netInput, nodeInput].forEach(function(input) {
            if (input) {
                input.addEventListener('input', function() {
                    if (this.value.trim() && fullAddressInput) {
                        fullAddressInput.value = '';
                    }
                });
            }
        });
    }
    
    // Add form validation
    const form = document.querySelector('form');
    if (form) {
        form.addEventListener('submit', function(e) {
            const inputs = form.querySelectorAll('input');
            let hasValue = false;
            
            inputs.forEach(function(input) {
                if (input.value.trim()) {
                    hasValue = true;
                }
            });
            
            if (!hasValue) {
                e.preventDefault();
                alert('Please enter at least one search criteria');
            }
        });
    }
});