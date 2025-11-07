#!/bin/bash

# Quick start script for Voice Agent

echo "================================================"
echo "Voice Agent - Quick Start"
echo "================================================"

# Check if .env exists
if [ ! -f .env ]; then
    echo "❌ .env file not found!"
    echo "Creating .env from .env.example..."
    cp .env.example .env
    echo "✅ .env created. Please edit it and add your OPENAI_API_KEY"
    echo ""
    echo "Edit .env and then run this script again."
    exit 1
fi

# Check if virtual environment exists
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
    echo "✅ Virtual environment created"
fi

# Activate virtual environment
echo "Activating virtual environment..."
source venv/bin/activate

# Install dependencies
echo "Installing dependencies..."
pip install -r requirements.txt

echo ""
echo "================================================"
echo "✅ Setup complete!"
echo "================================================"
echo ""
echo "To start the server:"
echo "  python main.py"
echo ""
echo "To test the API:"
echo "  python test_api.py"
echo ""
echo "API will be available at: http://localhost:8000"
echo "Documentation at: http://localhost:8000/docs"
echo ""
