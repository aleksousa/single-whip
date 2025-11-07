#!/usr/bin/env python3
"""
Simple script to test the Voice Agent API
"""
import requests
import json
import time


API_BASE_URL = "http://localhost:8000"


def test_health_check():
    """Test the health check endpoint"""
    print("\n=== Testing Health Check ===")
    response = requests.get(f"{API_BASE_URL}/")
    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")
    return response.status_code == 200


def test_join_room(room_id: str, system_prompt: str = None):
    """Test joining a room"""
    print(f"\n=== Joining Room {room_id} ===")

    payload = {"room_id": room_id}
    if system_prompt:
        payload["system_prompt"] = system_prompt

    response = requests.post(
        f"{API_BASE_URL}/join_room",
        json=payload
    )

    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")
    return response.status_code == 200


def test_list_rooms():
    """Test listing active rooms"""
    print("\n=== Listing Active Rooms ===")
    response = requests.get(f"{API_BASE_URL}/rooms")
    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")
    return response.status_code == 200


def test_leave_room(room_id: str):
    """Test leaving a room"""
    print(f"\n=== Leaving Room {room_id} ===")
    response = requests.post(f"{API_BASE_URL}/leave_room/{room_id}")
    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")
    return response.status_code == 200


def main():
    """Run all tests"""
    print("=" * 60)
    print("Voice Agent API Test Suite")
    print("=" * 60)

    # Test health check
    if not test_health_check():
        print("\n❌ Health check failed! Is the server running?")
        return

    # Test joining a room
    room_id = "test-123"
    system_prompt = "You are a friendly test assistant who likes to make jokes."

    if not test_join_room(room_id, system_prompt):
        print(f"\n❌ Failed to join room {room_id}")
        return

    # Wait a bit
    time.sleep(2)

    # List rooms
    test_list_rooms()

    # Wait a bit more
    time.sleep(2)

    # Leave room
    if not test_leave_room(room_id):
        print(f"\n❌ Failed to leave room {room_id}")
        return

    # List rooms again
    test_list_rooms()

    print("\n" + "=" * 60)
    print("✅ All tests completed!")
    print("=" * 60)


if __name__ == "__main__":
    try:
        main()
    except requests.exceptions.ConnectionError:
        print("\n❌ Could not connect to API server.")
        print("Make sure the server is running: python main.py")
    except Exception as e:
        print(f"\n❌ Error: {e}")
