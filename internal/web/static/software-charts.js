// Shared chart utilities for software analytics pages

const chartColors = [
    '#3498db', '#e74c3c', '#2ecc71', '#f39c12', '#9b59b6',
    '#1abc9c', '#34495e', '#e67e22', '#95a5a6', '#16a085',
    '#27ae60', '#2980b9', '#8e44ad', '#c0392b', '#d35400'
];

function createPieChart(canvasId, labels, data, title) {
    const ctx = document.getElementById(canvasId).getContext('2d');
    new Chart(ctx, {
        type: 'pie',
        data: {
            labels: labels,
            datasets: [{
                data: data,
                backgroundColor: chartColors
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    position: 'bottom',
                    labels: {
                        boxWidth: 12,
                        padding: 10
                    }
                },
                tooltip: {
                    callbacks: {
                        label: function(context) {
                            const label = context.label || '';
                            const value = context.parsed || 0;
                            const total = context.dataset.data.reduce((a, b) => a + b, 0);
                            const percentage = ((value / total) * 100).toFixed(1);
                            return label + ': ' + value + ' (' + percentage + '%)';
                        }
                    }
                }
            }
        }
    });
}

function createBarChart(canvasId, labels, data, title) {
    const ctx = document.getElementById(canvasId).getContext('2d');
    new Chart(ctx, {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [{
                label: 'Number of Nodes',
                data: data,
                backgroundColor: '#3498db',
                borderColor: '#2980b9',
                borderWidth: 1
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            scales: {
                y: {
                    beginAtZero: true
                }
            },
            plugins: {
                legend: {
                    display: false
                },
                tooltip: {
                    callbacks: {
                        label: function(context) {
                            const value = context.parsed.y || 0;
                            const total = data.reduce((a, b) => a + b, 0);
                            const percentage = ((value / total) * 100).toFixed(1);
                            return 'Nodes: ' + value + ' (' + percentage + '%)';
                        }
                    }
                }
            }
        }
    });
}

function displaySoftwareList(listElementId, items, nameField, countField) {
    const listEl = document.getElementById(listElementId);
    if (!listEl) return;
    items.forEach(function(item) {
        const li = document.createElement('li');
        li.innerHTML =
            '<span>' + item[nameField] + '</span>' +
            '<span>' +
                '<span class="percentage">' + item.percentage.toFixed(1) + '%</span>' +
                '<span class="version-badge">' + item[countField] + ' nodes</span>' +
            '</span>';
        listEl.appendChild(li);
    });
}
