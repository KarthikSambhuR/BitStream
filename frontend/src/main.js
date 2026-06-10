import './style.css';
import './app.css';

import logo from './assets/images/logo-universal.png';

document.querySelector('#app').innerHTML = `
    <div class="container">
        <div class="glow-orb top-left"></div>
        <div class="glow-orb bottom-right"></div>
        
        <div class="card">
            <div class="logo-wrapper">
                <img id="logo" class="logo-img" alt="Wails Logo">
            </div>
            
            <h1 class="title">Hello, World!</h1>
            <p class="subtitle">Welcome to your premium Wails & Vite desktop application.</p>
            
            <div class="action-box">
                <button class="action-btn" id="helloBtn">
                    <span>Get Started</span>
                    <svg class="arrow-icon" viewBox="0 0 24 24" width="24" height="24">
                        <path fill="currentColor" d="M16.172 11l-5.364-5.364 1.414-1.414L20 12l-7.778 7.778-1.414-1.414L16.172 13H4v-2z"/>
                    </svg>
                </button>
            </div>
            
            <div class="footer-status">
                <span class="status-dot"></span> Powered by Go & WebKit
            </div>
        </div>
    </div>
`;

document.getElementById('logo').src = logo;

const helloBtn = document.getElementById('helloBtn');
helloBtn.addEventListener('click', () => {
    // Show a beautiful ripple or pop alert
    const originalText = helloBtn.querySelector('span').innerText;
    helloBtn.querySelector('span').innerText = "Awesome!";
    helloBtn.classList.add('clicked');
    
    setTimeout(() => {
        helloBtn.querySelector('span').innerText = originalText;
        helloBtn.classList.remove('clicked');
    }, 1500);
});

