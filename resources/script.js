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
            onClose: function(selectedDates, dateStr, instance) {
                if (selectedDates.length > 0) {
                    document.getElementById("when").value = dateStr;
                }
            }
        });
    }
});
