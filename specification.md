# **Project: "One-Way" Work-to-Personal Calendar Sync**

## **1\. Overview**

This document outlines the technical specification for a tool that performs a one-way synchronization from a "Work" Google Calendar to a "Personal" Google Calendar. The tool is designed to work around admin restrictions by using API access instead of the standard calendar sharing UI.

The core principle is that the **Work calendar is the "source of truth."** The tool will create a new, read-only "Work Sync" calendar on the personal account, populated with filtered events from the work account. Any manual changes made to this "Work Sync" calendar will be overwritten.

## **2\. Core Functional Requirements**

### **2.1. Sync Source & Destination**

* **Source (Read):** User's "Work" Google Calendar.  
* **Destination (Write):** A **new, separate calendar** within the user's "Personal" Google Account.

### **2.2. Destination Calendar**

* The tool must check if a calendar named **"Work Sync"** exists in the personal account (using a stored ID for subsequent runs).  
* **On First Run:** If the calendar does not exist, the tool must **automatically create it** with the following properties:  
  * summary (Name): "Work Sync"  
  * backgroundColor (Color): Set to **"Grape"** (API Color ID: 7).  
* This calendar is considered "owned" by the tool. All events on it are considered disposable and manageable by the script.

### **2.3. Sync Frequency & Window**

* **Frequency:** The synchronization process must run on a schedule, **once per hour**.  
* **Sync Window:** The tool will only process events within a rolling **two-week window**: the current week (Mon-Sun) and the next week (Mon-Sun).  
* **Cleanup:** The tool **must** automatically delete any events it previously created on the "Work Sync" calendar whose end time is now before the start of this rolling window.

## **3\. Event Handling & Filter Logic**

### **3.1. Event Selection (What to Sync)**

The tool will process all events from the source calendar within the sync window and apply the following rules:

1. **All-Day Events:**  
   * **Sync:** All all-day events **ARE** synced.  
   * **"Out of Office" Exception:** This rule **includes** all-day "Out of Office" events.  
2. **Timed Events:**  
   * **Sync:** Sync any timed event where **any part** of the event (start or end) falls between **6:00 AM** and **12:00 AM (midnight)** in the calendar's local time.  
   * **"Out of Office" Exception:** Timed events marked as "Out of Office" **MUST BE SKIPPED** and not synced.  
3. **RSVP Status:**  
   * Sync all events regardless of the user's RSVP status (Yes, No, Maybe, or No Response).  
4. **Recurring Events:**  
   * The tool **must not** sync the recurring series itself.  
   * Instead, it must find the **individual instances** (expansions) of the recurring event that fall within the two-week sync window and sync them as individual, non-recurring events.

### **3.2. Event Data (How to Sync)**

When creating a new event on the "Work Sync" calendar:

* **Data to Copy:**  
  * summary (Title)  
  * description (Description)  
  * conferenceData (Meeting links, e.g., Google Meet, Zoom)  
  * start and end times  
  * location  
* **Data to Omit:**  
  * Guest List (attendees)  
  * Original event attachments  
  * All other non-essential metadata  
* **Properties on Creation:**  
  * **Reminders:** Use the "Work Sync" calendar's **default reminders**.  
  * **Notifications:** The Events: insert API call must **explicitly disable notifications** (sendUpdates="none") to prevent the user from being spammed on every sync.

## **4\. Architecture & Data Integrity**

### **4.1. Authentication**

* The tool will use **OAuth 2.0**.  
* It must manage **two (2)** separate refresh tokens:  
  1. **Work Account Token:** Requesting https://www.googleapis.com/auth/calendar.events.readonly scope.  
  2. **Personal Account Token:** Requesting https://www.googleapis.com/auth/calendar scope (full rights to create/manage calendars and events).

### **4.2. Idempotency (Preventing Duplicates)**

This is the most critical logic for preventing duplicate events and enabling updates.

* **Tracking:** The tool must use a **hidden tag**.  
* **On Create:** When syncing a work event, the tool must store the work event's id in the new personal event's extendedProperties.private.workEventId field.  
* **On Sync:** Before creating any event, the tool must first query the "Work Sync" calendar for an event with that workEventId in its extended properties.  
  * If **found**, proceed to **4.3 Update Logic**.  
  * If **not found**, proceed to create the event.

### **4.3. Update Logic (Source of Truth)**

* The Work calendar is the single source of truth.  
* If a synced event is found (per 4.2), the tool must check if its summary, description, start, or end times differ from the source work event.  
* If they differ, the tool **must overwrite** the event on the "Work Sync" calendar with the new data.  
* This logic automatically handles cases where the user manually modifies an event on the "Work Sync" calendar; their changes **will be reverted** on the next hourly sync.

### **4.4. Deletion Logic**

* **Source Deletion:** The tool must get a list of all events on the "Work Sync" calendar (identified by the hidden tag). If any of these events no longer exist on the source work calendar (within the sync window), they **must be deleted** from the "Work Sync" calendar.  
* **Stale Event Cleanup:** (See 2.3)

### **4.5. Error Handling & Reporting**

* **Run Mode:** The tool must run in **"Silent Mode."** It should not send any notifications on successful runs.  
* **Failure Mode:** If the tool fails (e.g., an API error, an expired auth token), it must send a **single email notification** to the user (e.g., their personal email) informing them that the sync has failed and requires attention (e.g., re-authentication).

## **5\. Testing Plan**

A developer must test the following scenarios to validate the implementation.

| Scenario | Action | Expected Outcome |
| :---- | :---- | :---- |
| **1\. First Run** | Run the tool for the first time. | "Work Sync" calendar is created with "Grape" color. Events from the two-week window are backfilled. No notifications are received. |
| **2\. Event Creation** | Add a new 10 AM meeting to the work calendar. | The event appears on the "Work Sync" calendar after the next hourly run. |
| **3\. Event Update** | Change the 10 AM meeting to 11 AM. | The synced event on "Work Sync" is automatically moved to 11 AM. |
| **4\. Event Deletion** | Delete the 11 AM meeting from work. | The synced event is deleted from "Work Sync". |
| **5\. Source of Truth** | Manually rename the 11 AM event on "Work Sync". | On the next run, the event's name is reverted to the original work title. |
| **6\. Stale Cleanup** | Manually run the script for a future window. | Old events (from 3+ weeks ago) are auto-deleted from "Work Sync". |
| **7\. Filter: All-Day OOF** | Create an all-day "Out of Office" event on work. | The event **IS** synced. |
| **8\. Filter: Timed OOF** | Create a 2 PM "Out of Office" event on work. | The event **IS NOT** synced. |
| **9\. Filter: Time Window** | Create a 5 AM event. | The event **IS NOT** synced. |
| **10\. Filter: Time Window** | Create a 5:30 AM \- 6:30 AM event. | The event **IS** synced. |
| **11\. Recurring Events** | Create a weekly meeting. | Only the **individual instances** in the two-week window are synced. The series itself is not. |
| **12\. Auth Failure** | Manually revoke one of the OAuth tokens. | The user receives **one** email notification that the sync has failed. |

