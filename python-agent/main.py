import asyncio
import threading
from flask import Flask, request, jsonify
from flask_cors import CORS
from pydantic import BaseModel, Field, ValidationError
from loguru import logger
from typing import Optional, Dict
import sys
import atexit

from config import settings
from webrtc_client import WhipClient
from voice_agent import SimpleVoiceAgent
from aiortc import MediaStreamTrack
from av import AudioFrame
import numpy as np

# Configure logging
logger.remove()
logger.add(sys.stderr, level="INFO")

app = Flask(__name__)
CORS(app)

# Store active sessions
active_sessions: Dict[str, dict] = {}

# Asyncio event loop in a separate thread
loop = None
loop_thread = None


def start_event_loop():
    """Start asyncio event loop in a separate thread"""
    global loop
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    loop.run_forever()


def run_async(coro):
    """Run async coroutine in the event loop thread"""
    return asyncio.run_coroutine_threadsafe(coro, loop).result()


# Start event loop thread
loop_thread = threading.Thread(target=start_event_loop, daemon=True)
loop_thread.start()


class JoinRoomRequest(BaseModel):
    """Request to join a room"""
    room_id: str = Field(..., description="Room ID to join")
    system_prompt: Optional[str] = Field(
        None,
        description="Custom system prompt for the agent. If not provided, uses default."
    )


class AudioTrackPlaceholder(MediaStreamTrack):
    """
    Placeholder audio track that generates silence
    In a full implementation, this would be replaced with actual audio from the agent
    """
    kind = "audio"

    def __init__(self):
        super().__init__()
        self.sample_rate = 48000
        self.channels = 2
        self.samples_per_frame = 960  # 20ms at 48kHz

    async def recv(self):
        """Generate silent audio frames"""
        # Generate 20ms of silence
        pts = 0
        time_base = 1 / self.sample_rate

        # Create silent audio frame
        samples = np.zeros((self.samples_per_frame, self.channels), dtype=np.int16)

        frame = AudioFrame.from_ndarray(samples, format='s16', layout='stereo')
        frame.sample_rate = self.sample_rate
        frame.pts = pts
        frame.time_base = time_base

        # Wait for 20ms
        await asyncio.sleep(0.02)

        return frame


@app.route("/", methods=["GET"])
def root():
    """Health check endpoint"""
    return jsonify({
        "service": "Voice Agent API",
        "status": "running",
        "active_sessions": len(active_sessions)
    })


@app.route("/join_room", methods=["POST"])
def join_room():
    """
    Join a room and start the voice agent

    This endpoint:
    1. Creates a WebRTC connection to the WHIP server
    2. Joins the specified room
    3. Starts a voice agent that will converse with anyone in the room
    """
    try:
        # Parse and validate request
        data = request.get_json()
        if not data:
            return jsonify({"error": "Request body is required"}), 400

        try:
            req = JoinRoomRequest(**data)
        except ValidationError as e:
            return jsonify({"error": str(e)}), 400

        room_id = req.room_id

        # Check if already in this room
        if room_id in active_sessions:
            return jsonify({
                "error": f"Agent already active in room {room_id}"
            }), 400

        logger.info(f"Joining room: {room_id}")

        # Run async operations
        async def join_room_async():
            # Create voice agent
            agent = SimpleVoiceAgent(system_prompt=req.system_prompt)
            await agent.start()

            # Create WebRTC client
            whip_client = WhipClient(room_id=room_id)

            # Create audio track (placeholder for now)
            audio_track = AudioTrackPlaceholder()

            # Set up callback for incoming audio
            async def on_remote_track(track):
                logger.info(f"Receiving audio from remote peer in room {room_id}")
                # Here you would process incoming audio through the agent
                # For now, just log that we're receiving audio
                while True:
                    try:
                        frame = await track.recv()
                        # Process audio with agent here
                        # response_audio = await agent.process_audio(frame)
                    except Exception as e:
                        logger.error(f"Error receiving audio: {e}")
                        break

            whip_client.set_on_track_callback(on_remote_track)

            # Connect to room
            success = await whip_client.connect(audio_track)

            if not success:
                await agent.stop()
                raise Exception("Failed to connect to room")

            # Store session
            active_sessions[room_id] = {
                "whip_client": whip_client,
                "agent": agent,
                "audio_track": audio_track
            }

            logger.info(f"Successfully joined room {room_id}")

        # Execute async function
        run_async(join_room_async())

        return jsonify({
            "success": True,
            "room_id": room_id,
            "message": f"Agent successfully joined room {room_id} and is ready to chat"
        }), 200

    except Exception as e:
        logger.error(f"Error joining room: {e}")
        return jsonify({"error": str(e)}), 500


@app.route("/leave_room/<room_id>", methods=["POST"])
def leave_room(room_id: str):
    """Leave a room and stop the agent"""
    if room_id not in active_sessions:
        return jsonify({
            "error": f"No active session in room {room_id}"
        }), 404

    try:
        async def leave_room_async():
            session = active_sessions[room_id]

            # Stop agent
            await session["agent"].stop()

            # Close WebRTC connection
            await session["whip_client"].close()

            # Remove session
            del active_sessions[room_id]

            logger.info(f"Left room {room_id}")

        # Execute async function
        run_async(leave_room_async())

        return jsonify({
            "success": True,
            "message": f"Left room {room_id}"
        }), 200

    except Exception as e:
        logger.error(f"Error leaving room: {e}")
        return jsonify({"error": str(e)}), 500


@app.route("/rooms", methods=["GET"])
def list_rooms():
    """List all active rooms"""
    return jsonify({
        "active_rooms": list(active_sessions.keys()),
        "count": len(active_sessions)
    })


def cleanup_on_shutdown():
    """Clean up on shutdown"""
    logger.info("Shutting down, closing all sessions...")

    async def cleanup_async():
        for room_id in list(active_sessions.keys()):
            try:
                session = active_sessions[room_id]
                await session["agent"].stop()
                await session["whip_client"].close()
            except Exception as e:
                logger.error(f"Error closing session {room_id}: {e}")

    try:
        run_async(cleanup_async())
    except Exception as e:
        logger.error(f"Error during cleanup: {e}")

    # Stop event loop
    if loop:
        loop.call_soon_threadsafe(loop.stop)


# Register cleanup handler
atexit.register(cleanup_on_shutdown)


if __name__ == "__main__":
    logger.info(f"Starting Voice Agent API on {settings.api_host}:{settings.api_port}")
    app.run(
        host=settings.api_host,
        port=settings.api_port,
        debug=True,
        threaded=True
    )
