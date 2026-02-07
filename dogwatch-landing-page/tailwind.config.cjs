module.exports = {
  content: [
    './index.html',
    './features.html',
    './docs.html',
    './downloads.html',
    './pricing.html'
  ],
  theme: {
    extend: {
      fontFamily: {
        mono: ['"Space Mono"', 'monospace'],
        display: ['Rajdhani', 'sans-serif']
      },
      colors: {
        'acid-green': '#ccff00',
        'off-black': '#050505',
        'signal-cyan': '#8be9fd',
        rust: '#ad4d2e',
        steel: '#8f8f8f',
        'dim-grid': '#1a1a1a'
      },
      backgroundImage: {
        'grid-pattern': 'linear-gradient(to right, #171717 1px, transparent 1px), linear-gradient(to bottom, #171717 1px, transparent 1px)'
      }
    }
  }
};
