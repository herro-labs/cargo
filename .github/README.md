# Cargo Framework CI/CD

This directory contains GitHub Actions workflows for automatically testing the Cargo framework on every commit and pull request.

## Workflows

### 🚀 `ci.yml` - Continuous Integration (Recommended)

**Simple, fast CI workflow** that runs on every push and pull request.

**Features:**
- ✅ Unit tests
- ✅ Integration tests with MongoDB
- ✅ Test coverage reporting
- ✅ Coverage threshold enforcement (90%)
- ✅ Automatic cleanup
- ✅ Fast execution (~3-5 minutes)

**Triggers:**
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop` branches

**Usage:**
```yaml
# Automatically runs on:
git push origin main
git push origin develop
# Opening/updating pull requests
```

### 🔧 `test.yml` - Comprehensive Test Suite

**Enterprise-grade testing workflow** with extensive validation.

**Features:**
- ✅ Unit tests on multiple Go versions (1.21, 1.22, 1.23)
- ✅ Integration tests with MongoDB
- ✅ Performance benchmarks
- ✅ Security scanning (gosec, govulncheck)
- ✅ Code quality checks (go vet, go fmt, staticcheck)
- ✅ Cross-platform build verification
- ✅ Dependency validation
- ✅ Comprehensive reporting
- ✅ PR comments with results

**Triggers:**
- Push to `main`, `develop`, `feature/*`, `hotfix/*` branches
- Pull requests to `main` or `develop` branches
- Manual triggering via GitHub UI

## Setup Instructions

### 1. Repository Configuration

**Required Secrets** (none currently needed, but recommended for production):
```bash
# Optional: For enhanced coverage reporting
CODECOV_TOKEN=your_codecov_token
```

**Branch Protection Rules** (recommended):
```yaml
Branch: main
- Require status checks to pass before merging
- Require branches to be up to date before merging
- Required status checks:
  - Run Tests (ci.yml)
  - Unit Tests (test.yml) 
  - Integration Tests (test.yml)
  - Test Coverage (test.yml)
```

### 2. Local Development Setup

**Prerequisites:**
```bash
# Install Go 1.21 or later
go version

# Install MongoDB (for integration tests)
docker run -d -p 27017:27017 mongo:7

# Install test dependencies
cd cargo
make test-deps
```

**Running Tests Locally:**
```bash
# Quick test run (same as CI)
make test-unit
make test-integration

# Full test suite
make test
make test-coverage

# Individual feature tests
make test-cache
make test-hooks
make test-config
```

## Workflow Details

### CI Workflow (`ci.yml`)

**Steps:**
1. **Checkout** - Get the latest code
2. **Setup Go** - Install Go 1.23
3. **Cache Dependencies** - Speed up subsequent runs
4. **Install Dependencies** - Install test tools
5. **Start MongoDB** - Launch MongoDB service
6. **Run Unit Tests** - Fast, isolated tests
7. **Run Integration Tests** - Tests with MongoDB
8. **Generate Coverage** - Create coverage report
9. **Check Coverage Threshold** - Enforce 90% minimum
10. **Upload Coverage** - Send to Codecov
11. **Cleanup** - Remove test data

**Performance:**
- **Duration**: ~3-5 minutes
- **Resource Usage**: 2 cores, 7GB RAM
- **Parallel Execution**: Yes (where possible)

### Comprehensive Test Workflow (`test.yml`)

**Jobs:**
1. **Unit Tests** - Run on Go 1.21, 1.22, 1.23 in parallel
2. **Integration Tests** - Full MongoDB integration
3. **Performance Benchmarks** - Speed and memory tests
4. **Test Coverage** - Coverage analysis and reporting
5. **Security Checks** - Security and vulnerability scanning
6. **Build Verification** - Cross-platform builds
7. **Dependency Checks** - Dependency validation
8. **Results Summary** - Aggregate results and PR comments

**Performance:**
- **Duration**: ~10-15 minutes
- **Resource Usage**: Multiple runners in parallel
- **Comprehensive Coverage**: All framework features

## Integration Features

### Coverage Reporting

**Codecov Integration:**
- Automatic coverage upload
- Coverage trends and reports
- PR coverage comments
- Coverage badges

**Setup Codecov:**
1. Visit [codecov.io](https://codecov.io)
2. Connect your GitHub repository
3. Add `CODECOV_TOKEN` to repository secrets (optional)

### PR Comments

**Automatic PR Comments:**
```markdown
## 🧪 Test Results

✅ **Unit Tests**: success
✅ **Integration Tests**: success  
✅ **Benchmarks**: success
✅ **Coverage**: success
✅ **Security Checks**: success
✅ **Build**: success
✅ **Dependency Checks**: success
```

### Status Checks

**GitHub Status Checks:**
- Unit Tests
- Integration Tests
- Coverage Threshold
- Security Scan
- Build Verification

## Troubleshooting

### Common Issues

**1. MongoDB Connection Failed**
```bash
# Check MongoDB service status
docker ps | grep mongo

# Restart MongoDB service  
docker restart <container_id>
```

**2. Coverage Below Threshold**
```bash
# Check current coverage
make test-coverage

# Identify uncovered code
open coverage.html
```

**3. Go Module Issues**
```bash
# Clean and reinstall
go clean -modcache
go mod download
```

**4. Test Timeout**
```bash
# Run with verbose output
go test -v -timeout 10m ./...
```

### Debugging Workflows

**View Workflow Logs:**
1. Go to GitHub repository
2. Click "Actions" tab
3. Select failed workflow run
4. Click on failed job
5. Expand step to view logs

**Local Testing:**
```bash
# Test exactly what CI runs
make test-deps
make test-unit
make test-integration
make test-coverage
make test-ci
```

## Performance Optimization

### Caching Strategy

**Go Module Cache:**
- Caches `~/go/pkg/mod` and `~/.cache/go-build`
- Key based on `go.sum` hash
- Reduces build time by ~30-50%

**MongoDB Cache:**
- Uses persistent MongoDB service
- Shared across test steps
- Cleaned up after each run

### Parallel Execution

**Unit Tests:**
- Run on 3 Go versions simultaneously
- Matrix strategy for parallelization
- ~3x faster than sequential

**Comprehensive Tests:**
- 8 jobs run in parallel
- Independent execution where possible
- ~5x faster than sequential

## Customization

### Adding New Tests

**Add to Makefile:**
```makefile
test-newfeature:
	go test -v ./test/unit/newfeature_test.go
```

**Add to CI workflow:**
```yaml
- name: Run new feature tests
  run: make test-newfeature
```

### Custom Environment Variables

**In workflow file:**
```yaml
env:
  CUSTOM_VAR: value
  MONGODB_URI: mongodb://localhost:27017/cargo_test
```

**In Makefile:**
```makefile
test-custom:
	CUSTOM_VAR=value go test ./...
```

## Monitoring

### Metrics Tracked

- **Test Duration**: Track execution time trends
- **Coverage Percentage**: Monitor coverage changes
- **Failure Rates**: Identify flaky tests
- **Performance Benchmarks**: Track performance regressions

### Alerts

**Recommended Alerts:**
- Coverage drops below 90%
- Tests fail on main branch
- Performance regression > 20%
- Security vulnerabilities detected

## Best Practices

### Commit Guidelines

**For optimal CI performance:**
1. **Small, focused commits** - Faster test execution
2. **Descriptive commit messages** - Better failure tracking
3. **Test locally first** - Reduce CI failures
4. **Update tests with code** - Maintain coverage

### Pull Request Guidelines

**Before creating PR:**
```bash
# Run full test suite locally
make test
make test-coverage
make test-ci

# Check code formatting
go fmt ./...
go vet ./...
```

**PR Description Template:**
```markdown
## Changes
- Brief description of changes

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass  
- [ ] Coverage maintained/improved
- [ ] Benchmarks reviewed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
```

This CI/CD setup ensures the Cargo framework maintains high quality, performance, and reliability standards automatically on every commit! 🚀 