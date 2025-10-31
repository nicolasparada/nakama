const ghosts = [
  {
    src: window.location.origin+"/images/ghost-01.png",
    size: { width: "80px", widthMd: "112px", height: "80px", heightMd: "112px" },
    animation: "move-around-1",
    duration: "45s"
  },
  {
    src: window.location.origin+"/images/ghost-02.png",
    size: { width: "64px", widthMd: "96px", height: "64px", heightMd: "96px" },
    animation: "move-around-2",
    duration: "60s"
  },
  {
    src: window.location.origin+"/images/ghost-01.png",
    size: { width: "48px", widthMd: "80px", height: "48px", heightMd: "80px" },
    animation: "move-around-3",
    duration: "75s"
  },
  {
    src: window.location.origin+"/images/ghost-02.png",
    size: { width: "96px", widthMd: "128px", height: "96px", heightMd: "128px" },
    animation: "move-around-4",
    duration: "50s"
  },
  {
    src: window.location.origin+"/images/ghost-01.png",
    size: { width: "56px", widthMd: "88px", height: "56px", heightMd: "88px" },
    animation: "move-around-1",
    duration: "80s"
  },
  {
    src: window.location.origin+"/images/ghost-02.png",
    size: { width: "40px", widthMd: "64px", height: "40px", heightMd: "64px" },
    animation: "move-around-2",
    duration: "90s"
  },
  {
    src: window.location.origin+"/images/ghost-01.png",
    size: { width: "112px", widthMd: "144px", height: "112px", heightMd: "144px" },
    animation: "move-around-3",
    duration: "40s"
  },
  {
    src: window.location.origin+"/images/ghost-02.png",
    size: { width: "64px", widthMd: "96px", height: "64px", heightMd: "96px" },
    animation: "move-around-4",
    duration: "65s"
  }
]

const cssContent = `
    .halloween-container {
        position: fixed;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        width: 100%;
        height: 100%;
        overflow: hidden;
        pointer-events: none;
        z-index: 10;
    }
    
    .halloween-ghost {
        position: absolute;
        top: 0;
        left: 0;
        opacity: 0.7;
    }
    
    .halloween-ghost img {
        width: 100%;
        height: 100%;
        object-fit: contain;
    }
    
    @keyframes move-around-1 {
        0% { transform: translate(-100%, 10vh) rotate(-10deg); }
        25% { transform: translate(25vw, 60vh) rotate(5deg); }
        50% { transform: translate(100vw, 40vh) rotate(-5deg); }
        75% { transform: translate(70vw, -10vh) rotate(10deg); }
        100% { transform: translate(-100%, 10vh) rotate(-10deg); }
    }
    @keyframes move-around-2 {
        0% { transform: translate(100vw, 80vh) rotate(10deg); }
        25% { transform: translate(70vw, 10vh) rotate(-5deg); }
        50% { transform: translate(-10vw, 5vh) rotate(5deg); }
        75% { transform: translate(20vw, 110vh) rotate(-10deg); }
        100% { transform: translate(100vw, 80vh) rotate(10deg); }
    }
    @keyframes move-around-3 {
        0% { transform: translate(20vw, -10vh) rotate(5deg); }
        25% { transform: translate(80vw, 40vh) rotate(-10deg); }
        50% { transform: translate(30vw, 110vh) rotate(10deg); }
        75% { transform: translate(-10vw, 70vh) rotate(-5deg); }
        100% { transform: translate(20vw, -10vh) rotate(5deg); }
    }
    @keyframes move-around-4 {
        0% { transform: translate(50vw, 110vh) rotate(-5deg); }
        25% { transform: translate(-10vw, 20vh) rotate(10deg); }
        50% { transform: translate(40vw, -10vh) rotate(-10deg); }
        75% { transform: translate(100vw, 50vh) rotate(5deg); }
        100% { transform: translate(50vw, 110vh) rotate(-5deg); }
    }
    
    @media (min-width: 768px) {
        .halloween-ghost-0 { width: 112px; height: 112px; }
        .halloween-ghost-1 { width: 96px; height: 96px; }
        .halloween-ghost-2 { width: 80px; height: 80px; }
        .halloween-ghost-3 { width: 128px; height: 128px; }
        .halloween-ghost-4 { width: 88px; height: 88px; }
        .halloween-ghost-5 { width: 64px; height: 64px; }
        .halloween-ghost-6 { width: 144px; height: 144px; }
        .halloween-ghost-7 { width: 96px; height: 96px; }
    }
`

export function initHalloween() {
    const style = document.createElement('style');
    style.type = 'text/css';
    style.appendChild(document.createTextNode(cssContent));
    document.head.appendChild(style);

    const container = document.createElement('div');
    container.className = "halloween-container";

    for (let i = 0; i < ghosts.length; i++) {
        const ghost = ghosts[i];
        const ghostDiv = document.createElement('div');
        ghostDiv.className = `halloween-ghost halloween-ghost-${i}`;
        ghostDiv.style.width = ghost.size.width;
        ghostDiv.style.height = ghost.size.height;
        ghostDiv.style.animation = `${ghost.animation} ${ghost.duration} linear infinite`;

        const img = document.createElement('img');
        img.src = ghost.src;
        img.alt = "Halloween Ghost";
        img.loading = "lazy";

        ghostDiv.appendChild(img);
        container.appendChild(ghostDiv);
    }

    document.body.appendChild(container);
}
