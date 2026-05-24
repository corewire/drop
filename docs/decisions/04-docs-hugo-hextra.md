# Feature: Automated Docs (Hugo Hextra)

## Goal
Use Hugo + Hextra to generate and publish operator documentation automatically.

## Plan
- Keep docs source in repository under a docs tree.
- Build docs with Hugo Hextra in CI.
- Publish docs site automatically from main branch/tag releases.
- Include versioned docs sections when release cadence requires it.

## Requirements
- Fast local preview command
- Broken-link checks in CI
