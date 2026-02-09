(function() {
    const container = document.getElementById('viewer-container');
    if (!container) return;

    const filesData = JSON.parse(container.dataset.files || '[]');
    if (filesData.length === 0) return;

    const width = container.clientWidth;
    const height = container.clientHeight;

    // Scene setup
    const scene = new THREE.Scene();
    scene.background = new THREE.Color(0x1f2937);

    const camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 10000);
    const renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setSize(width, height);
    renderer.setPixelRatio(window.devicePixelRatio);
    container.appendChild(renderer.domElement);

    // Lights
    const ambientLight = new THREE.AmbientLight(0x404040, 2);
    scene.add(ambientLight);

    const directionalLight = new THREE.DirectionalLight(0xffffff, 1.5);
    directionalLight.position.set(1, 1, 1);
    scene.add(directionalLight);

    const backLight = new THREE.DirectionalLight(0xffffff, 0.5);
    backLight.position.set(-1, -1, -1);
    scene.add(backLight);

    // Grid
    const gridHelper = new THREE.GridHelper(200, 50, 0x374151, 0x374151);
    scene.add(gridHelper);

    // Orbit controls
    let isMouseDown = false;
    let mouseX = 0, mouseY = 0;
    let rotX = 0.5, rotY = 0.5;
    let distance = 100;

    container.addEventListener('mousedown', (e) => {
        isMouseDown = true;
        mouseX = e.clientX;
        mouseY = e.clientY;
    });

    container.addEventListener('mousemove', (e) => {
        if (!isMouseDown) return;
        const dx = e.clientX - mouseX;
        const dy = e.clientY - mouseY;
        rotY += dx * 0.005;
        rotX += dy * 0.005;
        rotX = Math.max(-Math.PI / 2, Math.min(Math.PI / 2, rotX));
        mouseX = e.clientX;
        mouseY = e.clientY;
    });

    container.addEventListener('mouseup', () => { isMouseDown = false; });
    container.addEventListener('mouseleave', () => { isMouseDown = false; });

    container.addEventListener('wheel', (e) => {
        e.preventDefault();
        distance *= e.deltaY > 0 ? 1.1 : 0.9;
        distance = Math.max(1, Math.min(10000, distance));
    }, { passive: false });

    function updateCamera() {
        camera.position.x = distance * Math.cos(rotX) * Math.sin(rotY);
        camera.position.y = distance * Math.sin(rotX);
        camera.position.z = distance * Math.cos(rotX) * Math.cos(rotY);
        camera.lookAt(0, 0, 0);
    }

    // Model management
    let currentMesh = null;
    let currentIndex = 0;

    function clearModel() {
        if (currentMesh) {
            scene.remove(currentMesh);
            if (currentMesh.geometry) currentMesh.geometry.dispose();
            if (currentMesh.material) currentMesh.material.dispose();
            currentMesh = null;
        }
    }

    function addMesh(geometry) {
        const material = new THREE.MeshPhongMaterial({
            color: 0x6366f1,
            specular: 0x111111,
            shininess: 30,
            flatShading: false
        });
        const mesh = new THREE.Mesh(geometry, material);

        geometry.computeBoundingBox();
        const box = geometry.boundingBox;
        const center = new THREE.Vector3();
        box.getCenter(center);
        mesh.position.sub(center);

        const size = new THREE.Vector3();
        box.getSize(size);
        distance = Math.max(size.x, size.y, size.z) * 2;

        currentMesh = mesh;
        scene.add(mesh);

        const loading = document.getElementById('viewer-loading');
        if (loading) loading.remove();
    }

    function showLoading() {
        let loading = document.getElementById('viewer-loading');
        if (!loading) {
            loading = document.createElement('div');
            loading.id = 'viewer-loading';
            loading.className = 'absolute inset-0 flex items-center justify-center bg-gray-800/80';
            loading.innerHTML = '<div class="text-gray-400">Loading 3D model...</div>';
            container.appendChild(loading);
        }
    }

    function updateUI() {
        // Update counter
        const counter = document.getElementById('viewer-file-counter');
        if (counter) {
            counter.textContent = (currentIndex + 1) + '/' + filesData.length;
        }

        // Update tab highlight
        document.querySelectorAll('#file-tabs button[data-file-index]').forEach(btn => {
            const idx = parseInt(btn.dataset.fileIndex);
            if (idx === currentIndex) {
                btn.className = 'px-3 py-1 rounded text-xs font-medium whitespace-nowrap transition-colors bg-indigo-600 text-white';
            } else {
                btn.className = 'px-3 py-1 rounded text-xs font-medium whitespace-nowrap transition-colors bg-gray-700 text-gray-300 hover:bg-gray-600';
            }
        });

        // Update file list highlight in right panel
        document.querySelectorAll('[data-file-index]').forEach(el => {
            if (el.closest('#file-tabs')) return;
            const idx = parseInt(el.dataset.fileIndex);
            if (idx === currentIndex) {
                el.classList.add('bg-gray-700');
            } else {
                el.classList.remove('bg-gray-700');
            }
        });
    }

    function loadFile(index) {
        if (index < 0 || index >= filesData.length) return;
        currentIndex = index;
        clearModel();
        showLoading();
        updateUI();

        const file = filesData[index];
        if (file.ext === '.stl') {
            loadSTL(file.url);
        } else if (file.ext === '.obj') {
            loadOBJ(file.url);
        }
    }

    // STL loader
    function loadSTL(url) {
        const loader = new THREE.FileLoader();
        loader.setResponseType('arraybuffer');
        loader.load(url, function(data) {
            const geometry = parseSTL(data);
            addMesh(geometry);
        });
    }

    function parseSTL(data) {
        if (data.byteLength < 84) {
            return parseSTLASCII(new TextDecoder().decode(data));
        }

        const view = new DataView(data);
        const faceCount = view.getUint32(80, true);
        const expectedMinSize = 80 + 4 + faceCount * 50;

        // Binary if the face count is plausible (file is large enough to hold that many faces)
        if (faceCount > 0 && faceCount < 50000000 && expectedMinSize <= data.byteLength) {
            return parseSTLBinary(data, faceCount);
        }

        // Otherwise ASCII
        return parseSTLASCII(new TextDecoder().decode(data));
    }

    function parseSTLBinary(data, faceCount) {
        const view = new DataView(data);
        const vertices = new Float32Array(faceCount * 9);
        const normals = new Float32Array(faceCount * 9);

        let offset = 84;
        for (let i = 0; i < faceCount; i++) {
            const nx = view.getFloat32(offset, true); offset += 4;
            const ny = view.getFloat32(offset, true); offset += 4;
            const nz = view.getFloat32(offset, true); offset += 4;

            for (let j = 0; j < 3; j++) {
                const idx = i * 9 + j * 3;
                vertices[idx]     = view.getFloat32(offset, true); offset += 4;
                vertices[idx + 1] = view.getFloat32(offset, true); offset += 4;
                vertices[idx + 2] = view.getFloat32(offset, true); offset += 4;
                normals[idx]     = nx;
                normals[idx + 1] = ny;
                normals[idx + 2] = nz;
            }
            offset += 2;
        }

        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute('position', new THREE.BufferAttribute(vertices, 3));
        geometry.setAttribute('normal', new THREE.BufferAttribute(normals, 3));
        return geometry;
    }

    function parseSTLASCII(text) {
        const vertices = [];
        const normals = [];
        const lines = text.split('\n');
        let currentNormal = [0, 0, 0];

        for (const line of lines) {
            const trimmed = line.trim();
            if (trimmed.startsWith('facet normal')) {
                const parts = trimmed.split(/\s+/);
                currentNormal = [parseFloat(parts[2]), parseFloat(parts[3]), parseFloat(parts[4])];
            } else if (trimmed.startsWith('vertex')) {
                const parts = trimmed.split(/\s+/);
                vertices.push(parseFloat(parts[1]), parseFloat(parts[2]), parseFloat(parts[3]));
                normals.push(...currentNormal);
            }
        }

        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(vertices), 3));
        geometry.setAttribute('normal', new THREE.BufferAttribute(new Float32Array(normals), 3));
        return geometry;
    }

    // OBJ loader
    function loadOBJ(url) {
        const loader = new THREE.FileLoader();
        loader.load(url, function(text) {
            const geometry = parseOBJ(text);
            addMesh(geometry);
        });
    }

    function parseOBJ(text) {
        const verts = [];
        const norms = [];
        const positions = [];
        const normalArr = [];

        const lines = text.split('\n');
        for (const line of lines) {
            const parts = line.trim().split(/\s+/);
            if (parts[0] === 'v') {
                verts.push([parseFloat(parts[1]), parseFloat(parts[2]), parseFloat(parts[3])]);
            } else if (parts[0] === 'vn') {
                norms.push([parseFloat(parts[1]), parseFloat(parts[2]), parseFloat(parts[3])]);
            } else if (parts[0] === 'f') {
                const face = parts.slice(1).map(p => {
                    const indices = p.split('/');
                    return {
                        v: parseInt(indices[0]) - 1,
                        n: indices[2] ? parseInt(indices[2]) - 1 : -1
                    };
                });
                for (let i = 1; i < face.length - 1; i++) {
                    for (const idx of [face[0], face[i], face[i + 1]]) {
                        const v = verts[idx.v];
                        positions.push(v[0], v[1], v[2]);
                        if (idx.n >= 0 && norms[idx.n]) {
                            normalArr.push(norms[idx.n][0], norms[idx.n][1], norms[idx.n][2]);
                        } else {
                            normalArr.push(0, 0, 0);
                        }
                    }
                }
            }
        }

        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(positions), 3));
        geometry.setAttribute('normal', new THREE.BufferAttribute(new Float32Array(normalArr), 3));
        if (normalArr.every(n => n === 0)) {
            geometry.computeVertexNormals();
        }
        return geometry;
    }

    // Expose API
    window.viewer3d = {
        loadFile: loadFile,
        next: function() { loadFile((currentIndex + 1) % filesData.length); },
        prev: function() { loadFile((currentIndex - 1 + filesData.length) % filesData.length); }
    };

    // Event delegation for file tabs
    var fileTabs = document.getElementById('file-tabs');
    if (fileTabs) {
        fileTabs.addEventListener('click', function(e) {
            var btn = e.target.closest('[data-file-index]');
            if (btn) loadFile(parseInt(btn.dataset.fileIndex));
        });
    }

    // Event delegation for file list in right panel
    var fileList = document.getElementById('file-list');
    if (fileList) {
        fileList.addEventListener('click', function(e) {
            var item = e.target.closest('.file-list-item[data-file-index]');
            if (item) loadFile(parseInt(item.dataset.fileIndex));
        });
    }

    // Prev/next buttons
    var prevBtn = document.getElementById('viewer-prev-btn');
    var nextBtn = document.getElementById('viewer-next-btn');
    if (prevBtn) prevBtn.addEventListener('click', function() { loadFile((currentIndex - 1 + filesData.length) % filesData.length); });
    if (nextBtn) nextBtn.addEventListener('click', function() { loadFile((currentIndex + 1) % filesData.length); });

    // Load first file
    loadFile(0);

    // Animation loop
    function animate() {
        requestAnimationFrame(animate);
        updateCamera();
        renderer.render(scene, camera);
    }
    animate();

    // Handle resize
    window.addEventListener('resize', () => {
        const w = container.clientWidth;
        const h = container.clientHeight;
        camera.aspect = w / h;
        camera.updateProjectionMatrix();
        renderer.setSize(w, h);
    });
})();
