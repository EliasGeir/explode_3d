(function() {
    var container = document.getElementById('slicer-viewer');
    if (!container) return;

    var filesData = JSON.parse(container.dataset.files || '[]');
    var plateWidth = parseFloat(container.dataset.plateWidth) || 130;
    var plateDepth = parseFloat(container.dataset.plateDepth) || 80;
    var plateHeight = parseFloat(container.dataset.plateHeight) || 165;

    var width = container.clientWidth;
    var height = container.clientHeight;

    // Scene setup
    var scene = new THREE.Scene();
    scene.background = new THREE.Color(0x1f2937);

    var camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 10000);
    var renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setSize(width, height);
    renderer.setPixelRatio(window.devicePixelRatio);
    container.innerHTML = '';
    container.appendChild(renderer.domElement);

    // Lighting
    var ambientLight = new THREE.AmbientLight(0x404040, 2);
    scene.add(ambientLight);

    var dirLight = new THREE.DirectionalLight(0xffffff, 1.5);
    dirLight.position.set(1, 1, 1);
    scene.add(dirLight);

    var backLight = new THREE.DirectionalLight(0xffffff, 0.5);
    backLight.position.set(-1, -1, -1);
    scene.add(backLight);

    // Build plate grid
    var gridGroup = new THREE.Group();
    scene.add(gridGroup);

    function drawBuildPlate(w, d, h) {
        while (gridGroup.children.length > 0) {
            gridGroup.remove(gridGroup.children[0]);
        }

        var gridSize = Math.max(w, d);
        var divisions = Math.round(gridSize / 10);
        var grid = new THREE.GridHelper(gridSize, divisions, 0x374151, 0x2d3748);
        gridGroup.add(grid);

        var boxGeo = new THREE.BoxGeometry(w, h, d);
        var edges = new THREE.EdgesGeometry(boxGeo);
        var lineMat = new THREE.LineBasicMaterial({ color: 0x6366f1, transparent: true, opacity: 0.3 });
        var wireframe = new THREE.LineSegments(edges, lineMat);
        wireframe.position.set(0, h / 2, 0);
        gridGroup.add(wireframe);

        var plateGeo = new THREE.PlaneGeometry(w, d);
        var plateMat = new THREE.MeshBasicMaterial({
            color: 0x374151, transparent: true, opacity: 0.5, side: THREE.DoubleSide
        });
        var plate = new THREE.Mesh(plateGeo, plateMat);
        plate.rotation.x = -Math.PI / 2;
        plate.position.y = 0.01;
        gridGroup.add(plate);
    }

    drawBuildPlate(plateWidth, plateDepth, plateHeight);

    // Model materials
    var modelMaterial = new THREE.MeshPhongMaterial({
        color: 0x7c3aed, transparent: true, opacity: 0.85, shininess: 30, specular: 0x222222
    });
    var modelMaterialHover = new THREE.MeshPhongMaterial({
        color: 0x8b5cf6, transparent: true, opacity: 0.9, shininess: 40, specular: 0x333333
    });
    var modelMaterialDrag = new THREE.MeshPhongMaterial({
        color: 0xa78bfa, transparent: true, opacity: 0.7, shininess: 40, specular: 0x333333
    });

    var modelGroup = new THREE.Group();
    scene.add(modelGroup);

    // Raycaster for picking
    var raycaster = new THREE.Raycaster();
    var mouse = new THREE.Vector2();
    var dragPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0); // XZ plane at Y=0
    var dragOffset = new THREE.Vector3();
    var intersection = new THREE.Vector3();

    // Interaction state
    var hoveredMesh = null;
    var draggedMesh = null;
    var isOrbiting = false;
    var mouseX = 0, mouseY = 0;
    var mouseDownPos = { x: 0, y: 0 };
    var hasMoved = false;

    // Camera orbit
    var rotX = 0.6, rotY = 0.5;
    var distance = Math.max(plateWidth, plateDepth, plateHeight) * 2;
    var lookY = plateHeight * 0.3;

    function updateCamera() {
        camera.position.x = distance * Math.sin(rotY) * Math.cos(rotX);
        camera.position.y = lookY + distance * Math.sin(rotX);
        camera.position.z = distance * Math.cos(rotY) * Math.cos(rotX);
        camera.lookAt(0, lookY, 0);
    }
    updateCamera();

    function getMouseNDC(e) {
        var rect = renderer.domElement.getBoundingClientRect();
        mouse.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
        mouse.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
    }

    function getModelMeshes() {
        var meshes = [];
        modelGroup.traverse(function(child) {
            if (child.isMesh) meshes.push(child);
        });
        return meshes;
    }

    function hitTest(e) {
        getMouseNDC(e);
        raycaster.setFromCamera(mouse, camera);
        var hits = raycaster.intersectObjects(getModelMeshes(), false);
        return hits.length > 0 ? hits[0] : null;
    }

    function getPlaneIntersection(e) {
        getMouseNDC(e);
        raycaster.setFromCamera(mouse, camera);
        var target = new THREE.Vector3();
        if (raycaster.ray.intersectPlane(dragPlane, target)) {
            return target;
        }
        return null;
    }

    // --- Mouse events ---

    container.addEventListener('mousedown', function(e) {
        e.preventDefault();
        mouseDownPos.x = e.clientX;
        mouseDownPos.y = e.clientY;
        mouseX = e.clientX;
        mouseY = e.clientY;
        hasMoved = false;

        // Left click: try to pick a model for dragging
        if (e.button === 0) {
            var hit = hitTest(e);
            if (hit) {
                draggedMesh = hit.object;
                draggedMesh.material = modelMaterialDrag;

                // Set drag plane at the mesh's Y=0 (build plate level)
                dragPlane.set(new THREE.Vector3(0, 1, 0), 0);

                // Calculate offset between click point on plane and mesh position
                var planePoint = getPlaneIntersection(e);
                if (planePoint) {
                    dragOffset.copy(draggedMesh.position).sub(planePoint);
                }

                container.style.cursor = 'grabbing';
                return;
            }
        }

        // No model hit or right-click: orbit
        isOrbiting = true;
    });

    window.addEventListener('mousemove', function(e) {
        var dx = e.clientX - mouseX;
        var dy = e.clientY - mouseY;

        if (Math.abs(e.clientX - mouseDownPos.x) > 3 || Math.abs(e.clientY - mouseDownPos.y) > 3) {
            hasMoved = true;
        }

        if (draggedMesh) {
            // Drag model on XZ plane
            var planePoint = getPlaneIntersection(e);
            if (planePoint) {
                draggedMesh.position.x = planePoint.x + dragOffset.x;
                draggedMesh.position.z = planePoint.z + dragOffset.z;
                // Keep Y (vertical) unchanged
            }
            mouseX = e.clientX;
            mouseY = e.clientY;
            return;
        }

        if (isOrbiting) {
            rotY += dx * 0.01;
            rotX += dy * 0.01;
            rotX = Math.max(-Math.PI / 2 + 0.1, Math.min(Math.PI / 2 - 0.1, rotX));
            mouseX = e.clientX;
            mouseY = e.clientY;
            updateCamera();
            return;
        }

        // Hover effect
        var hit = hitTest(e);
        if (hit && hit.object !== draggedMesh) {
            if (hoveredMesh && hoveredMesh !== hit.object) {
                hoveredMesh.material = modelMaterial;
            }
            hoveredMesh = hit.object;
            hoveredMesh.material = modelMaterialHover;
            container.style.cursor = 'grab';
        } else {
            if (hoveredMesh) {
                hoveredMesh.material = modelMaterial;
                hoveredMesh = null;
            }
            container.style.cursor = 'default';
        }
    });

    window.addEventListener('mouseup', function(e) {
        if (draggedMesh) {
            draggedMesh.material = modelMaterial;
            draggedMesh = null;
            container.style.cursor = 'default';
        }
        isOrbiting = false;
    });

    container.addEventListener('contextmenu', function(e) { e.preventDefault(); });

    container.addEventListener('wheel', function(e) {
        distance *= e.deltaY > 0 ? 1.1 : 0.9;
        distance = Math.max(10, Math.min(2000, distance));
        updateCamera();
        e.preventDefault();
    }, { passive: false });

    // Animation loop
    function animate() {
        requestAnimationFrame(animate);
        renderer.render(scene, camera);
    }
    animate();

    // Resize handler
    window.addEventListener('resize', function() {
        var w = container.clientWidth;
        var h = container.clientHeight;
        if (w === 0 || h === 0) return;
        camera.aspect = w / h;
        camera.updateProjectionMatrix();
        renderer.setSize(w, h);
    });

    // STL parsing
    function parseSTLBinary(buffer) {
        var dv = new DataView(buffer);
        var faceCount = dv.getUint32(80, true);
        var vertices = new Float32Array(faceCount * 9);

        for (var i = 0; i < faceCount; i++) {
            var offset = 84 + i * 50;
            for (var v = 0; v < 3; v++) {
                var vOff = offset + 12 + v * 12;
                var sx = dv.getFloat32(vOff, true);
                var sy = dv.getFloat32(vOff + 4, true);
                var sz = dv.getFloat32(vOff + 8, true);
                // Convert: STL(x,y,z) -> Three.js(x, z, -y)
                vertices[i * 9 + v * 3]     = sx;
                vertices[i * 9 + v * 3 + 1] = sz;
                vertices[i * 9 + v * 3 + 2] = -sy;
            }
        }
        return vertices;
    }

    function loadSTLFile(url, callback) {
        var xhr = new XMLHttpRequest();
        xhr.open('GET', url, true);
        xhr.responseType = 'arraybuffer';
        xhr.onload = function() {
            if (xhr.status === 200) {
                callback(parseSTLBinary(xhr.response));
            }
        };
        xhr.send();
    }

    function addModelToScene(vertices) {
        var geometry = new THREE.BufferGeometry();
        geometry.setAttribute('position', new THREE.BufferAttribute(vertices, 3));
        geometry.computeVertexNormals();
        geometry.computeBoundingBox();

        var bbox = geometry.boundingBox;
        var cx = (bbox.max.x + bbox.min.x) / 2;
        var cz = (bbox.max.z + bbox.min.z) / 2;
        var minY = bbox.min.y;

        geometry.translate(-cx, -minY, -cz);

        var mesh = new THREE.Mesh(geometry, modelMaterial);
        modelGroup.add(mesh);

        // Fit camera
        geometry.computeBoundingBox();
        var size = geometry.boundingBox.getSize(new THREE.Vector3());
        var maxDim = Math.max(size.x, size.y, size.z);
        distance = maxDim * 2.5;
        lookY = size.y * 0.4;
        updateCamera();
    }

    // Load all files
    filesData.forEach(function(file) {
        if (file.url.toLowerCase().endsWith('.stl')) {
            loadSTLFile(file.url, addModelToScene);
        }
    });

    // Public API for profile changes
    window.slicerUpdateProfile = function(w, d, h) {
        plateWidth = w;
        plateDepth = d;
        plateHeight = h;
        drawBuildPlate(w, d, h);
        container.dataset.plateWidth = w;
        container.dataset.plateDepth = d;
        container.dataset.plateHeight = h;
    };
})();
