export function initTheme(THEME_TOGGLE, applyTheme, toggleTheme) {
    const savedTheme = localStorage.getItem('theme') || 'light';
    applyTheme(savedTheme, THEME_TOGGLE);
    THEME_TOGGLE.addEventListener('click', () => toggleTheme(THEME_TOGGLE, applyTheme));
}

export function applyTheme(theme, THEME_TOGGLE) {
    if (theme === 'dark') {
        document.body.classList.add('dark-mode');
        THEME_TOGGLE.textContent = '🌙';
    } else {
        document.body.classList.remove('dark-mode');
        THEME_TOGGLE.textContent = '☀️';
    }
}

export function toggleTheme(THEME_TOGGLE, applyTheme) {
    const currentTheme = document.body.classList.contains('dark-mode') ? 'dark' : 'light';
    const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
    localStorage.setItem('theme', newTheme);
    applyTheme(newTheme, THEME_TOGGLE);
} 