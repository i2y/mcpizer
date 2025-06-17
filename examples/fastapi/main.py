from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List, Optional
from datetime import datetime

app = FastAPI(
    title="Sample FastAPI Service",
    description="A sample API to test MCP Bridge auto-discovery",
    version="1.0.0"
)

# Models
class Item(BaseModel):
    id: int
    name: str
    description: Optional[str] = None
    price: float
    tax: Optional[float] = None
    created_at: datetime = datetime.now()

class ItemCreate(BaseModel):
    name: str
    description: Optional[str] = None
    price: float
    tax: Optional[float] = None

# In-memory storage
items_db: List[Item] = []
next_id = 1

# Routes
@app.get("/")
def read_root():
    return {"message": "Welcome to Sample FastAPI Service"}

@app.get("/items", response_model=List[Item])
def get_items(skip: int = 0, limit: int = 10):
    """Get all items with pagination"""
    return items_db[skip : skip + limit]

@app.get("/items/{item_id}", response_model=Item)
def get_item(item_id: int):
    """Get a specific item by ID"""
    for item in items_db:
        if item.id == item_id:
            return item
    raise HTTPException(status_code=404, detail="Item not found")

@app.post("/items", response_model=Item)
def create_item(item: ItemCreate):
    """Create a new item"""
    global next_id
    new_item = Item(
        id=next_id,
        name=item.name,
        description=item.description,
        price=item.price,
        tax=item.tax
    )
    items_db.append(new_item)
    next_id += 1
    return new_item

@app.put("/items/{item_id}", response_model=Item)
def update_item(item_id: int, item: ItemCreate):
    """Update an existing item"""
    for idx, existing_item in enumerate(items_db):
        if existing_item.id == item_id:
            updated_item = Item(
                id=item_id,
                name=item.name,
                description=item.description,
                price=item.price,
                tax=item.tax,
                created_at=existing_item.created_at
            )
            items_db[idx] = updated_item
            return updated_item
    raise HTTPException(status_code=404, detail="Item not found")

@app.delete("/items/{item_id}")
def delete_item(item_id: int):
    """Delete an item"""
    for idx, item in enumerate(items_db):
        if item.id == item_id:
            del items_db[idx]
            return {"message": "Item deleted successfully"}
    raise HTTPException(status_code=404, detail="Item not found")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
