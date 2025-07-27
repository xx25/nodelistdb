// Fancy Date Picker for NodelistDB Stats Page
class NodelistDatePicker {
    constructor(containerId, availableDates, currentDate) {
        this.container = document.getElementById(containerId);
        this.availableDates = new Set(availableDates);
        // Firefox-safe date parsing
        this.currentDate = this.parseDate(currentDate);
        this.selectedDate = this.parseDate(currentDate);
        this.viewDate = this.parseDate(currentDate);
        this.minDate = availableDates.length > 0 ? new Date(availableDates[availableDates.length - 1]) : null;
        this.maxDate = availableDates.length > 0 ? new Date(availableDates[0]) : null;
        
        this.monthNames = ['January', 'February', 'March', 'April', 'May', 'June',
                          'July', 'August', 'September', 'October', 'November', 'December'];
        this.dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
        
        this.init();
    }
    
    init() {
        this.render();
        this.attachEventListeners();
    }
    
    render() {
        const pickerHTML = `
            <div class="date-picker-wrapper">
                <div class="date-picker-input-group">
                    <input type="text" 
                           id="date-picker-input" 
                           class="date-picker-input" 
                           value="${this.formatDate(this.selectedDate)}" 
                           readonly>
                    <button type="button" class="date-picker-toggle" aria-label="Toggle calendar">
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <rect x="3" y="4" width="18" height="18" rx="2" ry="2"></rect>
                            <line x1="16" y1="2" x2="16" y2="6"></line>
                            <line x1="8" y1="2" x2="8" y2="6"></line>
                            <line x1="3" y1="10" x2="21" y2="10"></line>
                        </svg>
                    </button>
                </div>
                <div class="date-picker-dropdown" id="date-picker-dropdown" style="display: none;">
                    <div class="date-picker-header">
                        <button type="button" class="date-picker-nav date-picker-nav-prev-year" aria-label="Previous year">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                                <path d="M11.67 3.87L9.9 2.1 0 12l9.9 9.9 1.77-1.77L3.54 12z"/>
                                <path d="M18.67 3.87L16.9 2.1 7 12l9.9 9.9 1.77-1.77L10.54 12z"/>
                            </svg>
                        </button>
                        <button type="button" class="date-picker-nav date-picker-nav-prev" aria-label="Previous month">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                                <path d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z"/>
                            </svg>
                        </button>
                        <div class="date-picker-current">
                            <select class="date-picker-month-select" aria-label="Select month">
                                ${this.monthNames.map((month, i) => 
                                    `<option value="${i}" ${i === this.viewDate.getMonth() ? 'selected' : ''}>${month}</option>`
                                ).join('')}
                            </select>
                            <select class="date-picker-year-select" aria-label="Select year">
                                ${this.generateYearOptions()}
                            </select>
                        </div>
                        <button type="button" class="date-picker-nav date-picker-nav-next" aria-label="Next month">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                                <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z"/>
                            </svg>
                        </button>
                        <button type="button" class="date-picker-nav date-picker-nav-next-year" aria-label="Next year">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                                <path d="M12.33 20.13l1.77-1.77L22.46 12l-8.36-8.36-1.77 1.77L18.46 12z"/>
                                <path d="M5.33 20.13l1.77-1.77L15.46 12 7.1 3.64 5.33 5.41 11.46 12z"/>
                            </svg>
                        </button>
                    </div>
                    <div class="date-picker-days-header">
                        ${this.dayNames.map(day => `<div class="date-picker-day-name">${day}</div>`).join('')}
                    </div>
                    <div class="date-picker-days" id="date-picker-days">
                        ${this.generateCalendarDays()}
                    </div>
                    <div class="date-picker-footer">
                        <button type="button" class="date-picker-today-btn">Today</button>
                        <button type="button" class="date-picker-clear-btn">Latest Data</button>
                    </div>
                    <div class="date-picker-legend">
                        <span class="legend-item"><span class="legend-dot available"></span> Available</span>
                        <span class="legend-item"><span class="legend-dot selected"></span> Selected</span>
                        <span class="legend-item"><span class="legend-dot unavailable"></span> No Data</span>
                    </div>
                </div>
            </div>
        `;
        
        this.container.innerHTML = pickerHTML;
    }
    
    generateYearOptions() {
        const startYear = this.minDate ? this.minDate.getFullYear() : 2007;
        const endYear = this.maxDate ? this.maxDate.getFullYear() : new Date().getFullYear();
        const currentYear = this.viewDate.getFullYear();
        
        let options = '';
        for (let year = endYear; year >= startYear; year--) {
            options += `<option value="${year}" ${year === currentYear ? 'selected' : ''}>${year}</option>`;
        }
        return options;
    }
    
    generateCalendarDays() {
        const year = this.viewDate.getFullYear();
        const month = this.viewDate.getMonth();
        const firstDay = new Date(year, month, 1).getDay();
        const daysInMonth = new Date(year, month + 1, 0).getDate();
        const daysInPrevMonth = new Date(year, month, 0).getDate();
        
        let daysHTML = '';
        
        // Previous month days
        for (let i = firstDay - 1; i >= 0; i--) {
            const day = daysInPrevMonth - i;
            const date = new Date(year, month - 1, day);
            daysHTML += this.createDayElement(date, 'other-month');
        }
        
        // Current month days
        for (let day = 1; day <= daysInMonth; day++) {
            const date = new Date(year, month, day);
            daysHTML += this.createDayElement(date);
        }
        
        // Next month days
        const totalCells = Math.ceil((firstDay + daysInMonth) / 7) * 7;
        const remainingCells = totalCells - (firstDay + daysInMonth);
        for (let day = 1; day <= remainingCells; day++) {
            const date = new Date(year, month + 1, day);
            daysHTML += this.createDayElement(date, 'other-month');
        }
        
        return daysHTML;
    }
    
    createDayElement(date, additionalClass = '') {
        const dateStr = this.formatDate(date);
        const isAvailable = this.availableDates.has(dateStr);
        const isSelected = this.formatDate(date) === this.formatDate(this.selectedDate);
        const isToday = this.formatDate(date) === this.formatDate(new Date());
        
        let classes = ['date-picker-day'];
        if (additionalClass) classes.push(additionalClass);
        if (isAvailable) classes.push('available');
        if (isSelected) classes.push('selected');
        if (isToday) classes.push('today');
        if (!isAvailable) classes.push('unavailable');
        
        return `<button type="button" 
                        class="${classes.join(' ')}" 
                        data-date="${dateStr}"
                        data-day="${date.getDate()}"
                        ${!isAvailable ? 'disabled' : ''}
                        aria-label="${date.toLocaleDateString('en-US', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })}">
                    ${date.getDate()}
                </button>`;
    }
    
    formatDate(date) {
        const year = date.getFullYear();
        const month = String(date.getMonth() + 1).padStart(2, '0');
        const day = String(date.getDate()).padStart(2, '0');
        return `${year}-${month}-${day}`;
    }
    
    parseDate(dateStr) {
        // Firefox-safe date parsing from YYYY-MM-DD format
        const [year, month, day] = dateStr.split('-').map(num => parseInt(num, 10));
        return new Date(year, month - 1, day);
    }
    
    updateCalendar() {
        document.getElementById('date-picker-days').innerHTML = this.generateCalendarDays();
        document.querySelector('.date-picker-month-select').value = this.viewDate.getMonth();
        document.querySelector('.date-picker-year-select').value = this.viewDate.getFullYear();
    }
    
    attachEventListeners() {
        const dropdown = document.getElementById('date-picker-dropdown');
        const toggleBtn = document.querySelector('.date-picker-toggle');
        const input = document.getElementById('date-picker-input');
        
        // Toggle dropdown
        toggleBtn.addEventListener('click', () => {
            dropdown.style.display = dropdown.style.display === 'none' ? 'block' : 'none';
        });
        
        input.addEventListener('click', () => {
            dropdown.style.display = dropdown.style.display === 'none' ? 'block' : 'none';
        });
        
        // Close dropdown when clicking outside
        document.addEventListener('click', (e) => {
            if (!this.container.contains(e.target)) {
                dropdown.style.display = 'none';
            }
        });
        
        // Navigation buttons
        document.querySelector('.date-picker-nav-prev').addEventListener('click', () => {
            this.viewDate.setMonth(this.viewDate.getMonth() - 1);
            this.updateCalendar();
        });
        
        document.querySelector('.date-picker-nav-next').addEventListener('click', () => {
            this.viewDate.setMonth(this.viewDate.getMonth() + 1);
            this.updateCalendar();
        });
        
        document.querySelector('.date-picker-nav-prev-year').addEventListener('click', () => {
            this.viewDate.setFullYear(this.viewDate.getFullYear() - 1);
            this.updateCalendar();
        });
        
        document.querySelector('.date-picker-nav-next-year').addEventListener('click', () => {
            this.viewDate.setFullYear(this.viewDate.getFullYear() + 1);
            this.updateCalendar();
        });
        
        // Month/Year selects
        document.querySelector('.date-picker-month-select').addEventListener('change', (e) => {
            this.viewDate.setMonth(parseInt(e.target.value));
            this.updateCalendar();
        });
        
        document.querySelector('.date-picker-year-select').addEventListener('change', (e) => {
            this.viewDate.setFullYear(parseInt(e.target.value));
            this.updateCalendar();
        });
        
        // Day selection
        this.container.addEventListener('click', (e) => {
            if (e.target.classList.contains('date-picker-day') && !e.target.disabled) {
                const dateStr = e.target.dataset.date;
                // Firefox-safe date parsing
                const [year, month, day] = dateStr.split('-').map(num => parseInt(num, 10));
                this.selectedDate = new Date(year, month - 1, day);
                input.value = dateStr;
                dropdown.style.display = 'none';
                
                // Navigate to the selected date
                window.location.href = `/stats?date=${dateStr}`;
            }
        });
        
        // Today button
        document.querySelector('.date-picker-today-btn').addEventListener('click', () => {
            const today = this.formatDate(new Date());
            // Find nearest available date to today
            window.location.href = `/stats?date=${today}`;
        });
        
        // Latest data button
        document.querySelector('.date-picker-clear-btn').addEventListener('click', () => {
            window.location.href = '/stats';
        });
    }
}