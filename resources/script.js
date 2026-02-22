document.addEventListener('DOMContentLoaded', function() {
    const pickerToggleButton = document.getElementById("when-picker-toggle");

    if (pickerToggleButton) {
        pickerToggleButton.addEventListener('click', function(event) {
            event.preventDefault(); // Prevent default button action (form submission)
        });

        flatpickr(pickerToggleButton, {
            enableTime: true,
            dateFormat: "Y-m-d H:i",
            defaultHour: 18,
            appendTo: document.body, // Ensure it's appended to the body for better positioning
            onClose: function(selectedDates, dateStr, instance) {
                if (selectedDates.length > 0) {
                    document.getElementById("when").value = dateStr;
                }
            }
        });
    }

    // Handle edit-in-place for config values
    document.querySelectorAll('.editable-value-display').forEach(displaySpan => {
        const keyName = displaySpan.id.replace('display-', '');
        const inputField = document.getElementById(`input-${keyName}`);
        const originalTextContentNode = displaySpan.firstChild; // Assuming text is first child

        displaySpan.addEventListener('click', function() {
            displaySpan.classList.add('d-none'); // Hide display span
            inputField.classList.remove('d-none'); // Show input field
            inputField.focus(); // Focus the input
            inputField.select(); // Select text for easy editing
        });

        // Hide input and show display on blur
        inputField.addEventListener('blur', function() {
            inputField.classList.add('d-none'); // Hide input field
            originalTextContentNode.nodeValue = inputField.value; // Update display text node
            displaySpan.classList.remove('d-none'); // Show display span
        });

        // Hide input and show display on Enter key
        inputField.addEventListener('keydown', function(event) {
            if (event.key === 'Enter') {
                event.preventDefault(); // Prevent form submission
                inputField.blur(); // Trigger blur event
            }
        });
    });

    // Function to check if any delete checkbox is selected
    function areAnyDeleteCheckboxesChecked() {
        const deleteCheckboxes = document.querySelectorAll('input[name="deleteKeys"]');
        for (const checkbox of deleteCheckboxes) {
            if (checkbox.checked) {
                return true;
            }
        }
        return false;
    }

    // Function to update the delete button's disabled state
    function updateDeleteButtonState() {
        const deleteButton = document.querySelector('button[name="action"][value="delete"]');
        if (deleteButton) {
            deleteButton.disabled = !areAnyDeleteCheckboxesChecked();
        }
    }

    // Initialize and add event listeners for delete button state
    const deleteCheckboxes = document.querySelectorAll('input[name="deleteKeys"]');
    if (deleteCheckboxes.length > 0) { // Only proceed if there are delete checkboxes
        // Initial state
        updateDeleteButtonState();

        // Add change listener to each checkbox
        deleteCheckboxes.forEach(checkbox => {
            checkbox.addEventListener('change', updateDeleteButtonState);
        });
    }
});