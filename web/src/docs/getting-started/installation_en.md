# Installation and Setup

## System Requirements

- **Operating System**: Linux, macOS, Windows
- **Memory**: minimum 4GB RAM
- **Disk Space**: minimum 2GB free space
- **Network**: internet access for downloading dependencies

## Installing Stroppy Cloud Panel

### 1. Download

```bash
# Download the latest version
curl -L https://github.com/stroppy-io/stroppy-cloud-panel/releases/latest/download/stroppy-cloud-panel.tar.gz | tar -xz

# Or clone the repository
git clone https://github.com/stroppy-io/stroppy-cloud-panel.git
cd stroppy-cloud-panel
```

### 2. Install Dependencies

```bash
# Install Node.js (for frontend)
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs

# Install Go (for backend)
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### 3. Build the Project

```bash
# Build web UI
cd web
yarn install
yarn build
cd ..

# Build backend
make build
```

### 4. Run

```bash
# Run backend
make run

# Run web UI (in separate terminal)
cd web
yarn dev
```

## Initial Setup

After installation, open your browser and navigate to `http://localhost:5173` (dev server) or `http://localhost:8080` if running through Docker compose.

### Creating the First User

1. Click "Register"
2. Fill out the registration form
3. Confirm email (if email is configured)
4. Log in to the system

## Next Steps

- [Creating First Configuration](../configuration/basic-config.md)
- [Running First Test](../getting-started/first-test.md)
- [Analyzing Results](../getting-started/analyzing-results.md)
