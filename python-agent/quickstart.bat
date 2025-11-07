@echo off

echo ================================================
echo Voice Agent - Quick Start
echo ================================================
echo.

REM Check if .env exists
if not exist .env (
    echo ❌ .env file not found!
    echo Creating .env from .env.example...
    copy .env.example .env
    echo ✅ .env created. Please edit it and add your OPENAI_API_KEY
    echo.
    echo Edit .env and then run this script again.
    pause
    exit /b 1
)

REM Check if virtual environment exists
if not exist venv (
    echo Creating virtual environment...
    python -m venv venv
    echo ✅ Virtual environment created
)

REM Activate virtual environment
echo Activating virtual environment...
call venv\Scripts\activate.bat

REM Install dependencies
echo Installing dependencies...
pip install -r requirements.txt

echo.
echo ================================================
echo ✅ Setup complete!
echo ================================================
echo.
echo To start the server:
echo   python main.py
echo.
echo To test the API:
echo   python test_api.py
echo.
echo API will be available at: http://localhost:8000
echo Documentation at: http://localhost:8000/docs
echo.
pause
