import './style.css';

// Imports GO funcetions
import { LoadAppList, StartAnalysis, SearchWild, SelectItem, LoadFromPhone, OpenUrl } from '../wailsjs/go/main/App';



// Listen for the emitUdid event from Go and set the UDID in the menu
window.runtime.EventsOn('deviceInfo', (info) => {
    document.getElementById('menu-udid').innerText = info.Udid || 'Unknown';
    document.getElementById('menu-model').innerText = info.Model || '-';
    document.getElementById('menu-os').innerText = info.OSVersion || '-';
    document.getElementById('menu-name').innerText = info.DeviceName || '-';

    // Change the color of the phone icon
    const phoneIndicator = document.getElementById('phoneIndicator');
    if (info.Connected === 'true') {
        phoneIndicator.classList.remove('offline');
        phoneIndicator.classList.add('online');
        phoneIndicator.setAttribute('aria-label', 'Device Connected');
    } else {
        phoneIndicator.classList.remove('online');
        phoneIndicator.classList.add('offline');
        phoneIndicator.setAttribute('aria-label', 'Device Disconnected');
    }
});

window.copyToClipboard = function () {
    const text = document.getElementById('menu-udid').innerText;
    window.runtime.ClipboardSetText(text).catch((err) => {
        console.error('Error copying to clipboard:', err);
    });
};


// The ItunesSearch function called when clicking the search button in the UI. It gets the search term from the input field, calls the Go function, and displays the results in a table.
window.SearchWild = function () {
    try {
        const searchInput = document.getElementById('search-input');
        const term = searchInput.value;
        SearchWild(term).then((raw) => {
            // Parse the JSON string returned from Go
            const result = JSON.parse(raw);
            const container = document.getElementById('search-results');
            const items = Array.isArray(result?.results) ? result.results : [];
            container.innerHTML = 'Searching...';
            setTimeout(() => {
                container.innerHTML = items.length
                    ? `<table id="itunes-results-table" class="itunes-results-table">
                         <thead>
                           <tr>
                             <th>Logo</th>
                             <th>Title</th>
                             <th>Info</th>
                           </tr>
                         </thead>
                         <tbody>
                           ${items.map(item => `
                             <tr class="result-item" data-track-id="${item.trackId}" data-bundle-id="${item.bundleId}" data-track-name="${item.trackName}">
                               <td class="result-logo">
                                 <img src="${item.artworkUrl60 || ''}" alt="${item.trackName} logo" />
                               </td>
                               <td class="result-title">${item.trackName}</td>
                               <td class="result-meta">${item.bundleId}  -  ${item.sellerName}</td>
                             </tr>
                           `).join('')}
                         </tbody>
                       </table>`
                    : 'No results found.';

                // Add event listener for clicks on result items
                container.addEventListener('click', (e) => {
                    const row = e.target.closest('.result-item');
                    if (row) {
                        const trackName = row.dataset.trackName || row.querySelector('.result-title').innerText;
                        const trackId = row.dataset.trackId;
                        const bundleId = row.dataset.bundleId;
                        window.selectItem(trackName, trackId, bundleId);
                    }
                });
            }, 200); // Adjust the timeout as needed
        });
    } catch (err) {
        console.error(err);
    }
};

// The LoadFromPhone function called when clicking the "Load App from Phone" button in the UI. It calls the Go function LoadFromPhone that retrieves app data from the connected phone, and displays the results in a table similar to ItunesSearch.
window.loadFromPhone = function () {
    try {
        LoadFromPhone().then((raw) => {
            // Parse the JSON string returned from Go
            const result = JSON.parse(raw);
            const container = document.getElementById('search-results');
            const items = result.flatMap(item => Array.isArray(item.results) ? item.results : []);
            container.innerHTML = 'Loading...';
            setTimeout(() => {
                container.innerHTML = items.length
                    ? `<table id="itunes-results-table" class="itunes-results-table">
                         <thead>
                           <tr>
                             <th>Logo</th>
                             <th>Title</th>
                             <th>Info</th>
                           </tr>
                         </thead>
                         <tbody>
                           ${items.map(item => `
                             <tr class="result-item" data-track-id="${item.trackId}" data-bundle-id="${item.bundleId}" data-track-name="${item.trackName}">
                               <td class="result-logo">
                                 <img src="${item.artworkUrl60 || ''}" alt="${item.trackName} logo" />
                               </td>
                               <td class="result-title">${item.trackName}</td>
                               <td class="result-meta">${item.bundleId}  -  ${item.sellerName}</td>
                             </tr>
                           `).join('')}
                         </tbody>
                       </table>`
                    : 'No results found.';

                // Add event listener for clicks on result items
                container.addEventListener('click', (e) => {
                    const row = e.target.closest('.result-item');
                    if (row) {
                        const trackName = row.dataset.trackName || row.querySelector('.result-title').innerText;
                        const trackId = row.dataset.trackId;
                        const bundleId = row.dataset.bundleId;
                        window.selectItem(trackName, trackId, bundleId);
                    }
                });
            }, 200); // Adjust the timeout as needed
        });
    } catch (err) {
        console.error(err);
    }
};


// Function to handle item selection
window.selectItem = function (trackName, trackId, bundleId) {
    try {
        SelectItem(trackName, trackId, bundleId);
        // removes the search results
        document.getElementById('search-results').innerHTML = '';
        // sets the search input to the selected item's trackName
        document.getElementById('search-input').value = trackName + ' (' + bundleId + ')';
    } catch (err) {
        console.error(err);
    }
};


// Setup the LoadAppList function
window.LoadAppList = function () {
    try {
        LoadAppList()
    } catch (err) {
        console.error(err);
    }
};

// Setup the StartAnalysis function
window.StartAnalysis = function () {
    try {
        StartAnalysis()
    } catch (err) {
        console.error(err);
    }
};

// Listen for the analysisStatus event from Go and update the analysis status in the UI,
// AnalysisStatus{
// 		Stage:   stage,
// 		Message: message,
// 		Percent: percent,
// 	})
window.runtime.EventsOn('analysisStatus', (status) => {
    const statusElement = document.getElementById('analysis-status');
    const bar = document.getElementById('analysis-progress-bar');
    const wrapper = document.querySelector('.analysis-progress');

    if (statusElement) {
        statusElement.innerText = status?.message ?? '';
    }
    if (wrapper && wrapper.classList.contains('hidden')) {
        wrapper.classList.remove('hidden');
    }
    if (bar) {
        const pct = Math.max(0, Math.min(100, Number(status?.percent ?? 0)));
        bar.style.width = `${pct}%`;
    }
    if (wrapper) {
        wrapper.setAttribute('aria-valuenow', String(Math.max(0, Math.min(100, Number(status?.percent ?? 0)))));
    }
});

window.OpenGoogle = function () {
    try {
        OpenUrl("https://www.google.com")
    } catch (err) {
        console.error(err);
    }
};



/* 
// Not in use

// Setup the printMessage function
window.printMessage = function () {
    try {
        PrintMessage()
    } catch (err) {
        console.error(err);
    }
};

window.openBrowser = function () {
    try {
        OpenBrowser()
    } catch (err) {
        console.error(err);
    }
};

function openNav() {
  document.getElementById("mySidenav").style.width = "250px";
}

function closeNav() {
  document.getElementById("mySidenav").style.width = "0";
}  
*/