# Feature: Advanced Debugging with Kamera

## Goal
Evaluate simulation-based verification for controller logic.

## Inputs
- https://github.com/tgoodwin/Kamera
- https://thenewstack.io/kamera-uses-simulation-to-verify-kubernetes-controller-logic/

## Plan
1. Create a small proof-of-concept for one reconciliation path.
2. Compare confidence/coverage with existing unit/integration tests.
3. Decide whether to adopt Kamera for regression suites.

## Exit criteria
- Clear recommendation: adopt now, adopt later, or decline.
- Documented tradeoffs (maintenance cost, learning curve, CI runtime impact).
